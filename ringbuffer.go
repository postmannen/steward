// Info: The idea about the ring buffer is that we have a FIFO
// buffer where we store all incomming messages requested by
// operators.
// Each message in process or waiting to be processed will be
// stored in a DB. When the processing of a given message is
// done it will be removed from the state db, and an entry will
// made in the persistent message log.

package steward

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	bolt "go.etcd.io/bbolt"
)

// samValue represents one message with a subject. This
// struct type is used when storing and retreiving from
// db.
type samDBValue struct {
	ID   int
	Data subjectAndMessage
}

// ringBuffer holds the data of the buffer,
type ringBuffer struct {
	// In memory buffer for the messages.
	bufData chan samDBValue
	// The database to use.
	db *bolt.DB
	// The current number of items in the database.
	totalMessagesIndex int
	mu                 sync.Mutex
	// The channel to send messages that have been processed,
	// and we want to store it in the permanent message log.
	permStore chan string
	// Name of node.
	nodeName Node
	// newMessagesCh from *server are also implemented here,
	// so the ringbuffer can send it's error messages the same
	// way as all messages are handled.
	newMessagesCh chan []subjectAndMessage
	metrics       *metrics
}

// newringBuffer returns a push/pop storage for values.
func newringBuffer(metrics *metrics, c Configuration, size int, dbFileName string, nodeName Node, newMessagesCh chan []subjectAndMessage) *ringBuffer {
	// Check if socket folder exists, if not create it
	if _, err := os.Stat(c.DatabaseFolder); os.IsNotExist(err) {
		err := os.MkdirAll(c.DatabaseFolder, 0700)
		if err != nil {
			log.Printf("error: failed to create database directory %v: %v\n", c.DatabaseFolder, err)
			os.Exit(1)
		}
	}

	DatabaseFilepath := filepath.Join(c.DatabaseFolder, dbFileName)

	// ---

	db, err := bolt.Open(DatabaseFilepath, 0600, nil)
	if err != nil {
		log.Printf("error: failed to open db: %v\n", err)
		os.Exit(1)
	}

	// Set up the metrics to be used.
	metrics.promMessagesProcessedTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "messages_processed_total",
		Help: "The last processed db in key value/store",
	})
	metrics.promRegistry.MustRegister(metrics.promMessagesProcessedTotal)

	metrics.promRingbufferStalledMessagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ringbuffer_stalled_messages_total",
		Help: "Number of stalled messages in ringbuffer",
	})
	metrics.promRegistry.MustRegister(metrics.promRingbufferStalledMessagesTotal)

	metrics.promInMemoryBufferMessagesCurrent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "in_memory_buffer_messages_current",
		Help: "The current value of messages in memory buffer",
	})
	metrics.promRegistry.MustRegister(metrics.promInMemoryBufferMessagesCurrent)

	return &ringBuffer{
		bufData:       make(chan samDBValue, size),
		db:            db,
		permStore:     make(chan string),
		nodeName:      nodeName,
		newMessagesCh: newMessagesCh,
		metrics:       metrics,
	}
}

// start will process incomming messages through the inCh,
// put the messages on a buffered channel
// and deliver messages out when requested on the outCh.
func (r *ringBuffer) start(inCh chan subjectAndMessage, outCh chan samDBValueAndDelivered, defaultMessageTimeout int, defaultMessageRetries int) {

	// Starting both writing and reading in separate go routines so we
	// can write and read concurrently.

	const samValueBucket string = "samValueBucket"
	const indexValueBucket string = "indexValueBucket"

	r.totalMessagesIndex = r.getIndexValue(indexValueBucket)

	// Fill the buffer when new data arrives into the system
	go r.fillBuffer(inCh, samValueBucket, indexValueBucket, defaultMessageTimeout, defaultMessageRetries)

	// Start the process to permanently store done messages.
	go r.startPermanentStore()

	// Start the process that will handle messages present in the ringbuffer.
	go r.processBufferMessages(samValueBucket, outCh)
}

// fillBuffer will fill the buffer in the ringbuffer  reading from the inchannel.
// It will also store the messages in a K/V DB while being processed.
func (r *ringBuffer) fillBuffer(inCh chan subjectAndMessage, samValueBucket string, indexValueBucket string, defaultMessageTimeout int, defaultMessageRetries int) {
	// At startup get all the values that might be in the K/V store so we can
	// put them into the buffer before we start to fill up with new incomming
	// messages to the system.
	// This is needed when the program have been restarted, and we need to check
	// if there where previously unhandled messages that need to be handled first.

	func() {
		s, err := r.dumpBucket(samValueBucket)
		if err != nil {
			er := fmt.Errorf("info: fillBuffer: retreival of values from k/v store failed, probaly empty database, and no previous entries in db to process: %v", err)
			log.Printf("%v\n", er)
			return
			//sendErrorLogMessage(r.newMessagesCh, node(r.nodeName), er)
		}

		for _, v := range s {
			r.bufData <- v
		}
	}()

	// Prepare the map structure to know what values are allowed
	// for the commands or events
	var coe CommandOrEvent
	coeAvailable := coe.GetCommandOrEventAvailable()
	coeAvailableValues := []CommandOrEvent{}
	for v := range coeAvailable.topics {
		coeAvailableValues = append(coeAvailableValues, v)
	}

	// Check for incomming messages. These are typically comming from
	// the go routine who reads the socket.
	for v := range inCh {

		// Check if the command or event exists in commandOrEvent.go
		if !coeAvailable.CheckIfExists(v.CommandOrEvent, v.Subject) {
			er := fmt.Errorf("error: fillBuffer: the event or command type do not exist, so this message will not be put on the buffer to be processed. Check the syntax used in the json file for the message. Allowed values are : %v, where given: coe=%v, with subject=%v", coeAvailableValues, v.CommandOrEvent, v.Subject)
			sendErrorLogMessage(r.newMessagesCh, Node(r.nodeName), er)

			fmt.Println()
			// if it was not a valid value, we jump back up, and
			// continue the range iteration.
			continue
		}

		// Check if message values for timers override default values
		if v.Message.ACKTimeout < 1 {
			v.Message.ACKTimeout = defaultMessageTimeout
		}
		if v.Message.Retries < 1 {
			v.Message.Retries = defaultMessageRetries
		}

		// --- Store the incomming message in the k/v store ---

		// Get a unique number for the message to use when storing
		// it in the databases, and also use when further processing.
		r.mu.Lock()
		dbID := r.totalMessagesIndex
		r.mu.Unlock()

		// Create a structure for JSON marshaling.
		samV := samDBValue{
			ID:   dbID,
			Data: v,
		}

		js, err := json.Marshal(samV)
		if err != nil {
			er := fmt.Errorf("error:fillBuffer: json marshaling: %v", err)
			sendErrorLogMessage(r.newMessagesCh, Node(r.nodeName), er)
		}

		// Store the incomming message in key/value store
		err = r.dbUpdate(r.db, samValueBucket, strconv.Itoa(dbID), js)
		if err != nil {
			er := fmt.Errorf("error: dbUpdate samValue failed: %v", err)
			sendErrorLogMessage(r.newMessagesCh, Node(r.nodeName), er)

		}

		// Put the message on the inmemory buffer.
		r.bufData <- samV

		// Increment index, and store the new value to the database.
		r.mu.Lock()
		r.totalMessagesIndex++
		r.dbUpdate(r.db, indexValueBucket, "index", []byte(strconv.Itoa(r.totalMessagesIndex)))
		r.mu.Unlock()
	}

	// When done close the buffer channel
	close(r.bufData)
}

// processBufferMessages will pick messages from the buffer, and process them
// one by one. The messages will be delivered on the outCh, and it will wait
// until a signal is received on the done channel before it continues with the
// next message.
func (r *ringBuffer) processBufferMessages(samValueBucket string, outCh chan samDBValueAndDelivered) {
	// Range over the buffer of messages to pass on to processes.
	for v := range r.bufData {
		r.metrics.promInMemoryBufferMessagesCurrent.Set(float64(len(r.bufData)))

		// Create a done channel per message. A process started by the
		// spawnProcess function will handle incomming messages sequentaly.
		// So in the spawnProcess function we put a struct{} value when a
		// message is processed on the "done" channel and an ack is received
		// for a message, and we wait here for the "done" to be received.

		// We start the actual processing of an individual message here within
		// it's own go routine. Reason is that we don't want to block other
		// messages to be processed while waiting for the done signal, or if an
		// error with an individual message occurs.
		go func(v samDBValue) {
			v.Data.Message.done = make(chan struct{})
			delivredCh := make(chan struct{})

			// Prepare the structure with the data, and a function that can
			// be called when the data is received for signaling back.
			sd := samDBValueAndDelivered{
				samDBValue: v,
				delivered: func() {
					delivredCh <- struct{}{}
				},
			}

			outCh <- sd
			// Just to confirm here that the message was picked up, to know if the
			// the read process have stalled or not.
			// For now it will not do anything,
			select {
			case <-delivredCh:
				// OK.
			case <-time.After(time.Second * 5):
				// TODO: Check out if more logic should be made here if messages are stuck etc.
				// Testing with a timeout here to figure out if messages are stuck
				// waiting for done signal.
				log.Printf("Error: *** message %v seems to be stuck, did not receive delivered signal from reading process\n", v.ID)

				r.metrics.promRingbufferStalledMessagesTotal.Inc()
			}
			// Listen on the done channel here , so a go routine handling the
			// message will be able to signal back here that the message have
			// been processed, and that we then can delete it out of the K/V Store.

			select {
			case <-v.Data.done:
				log.Printf("info: processBufferMessages: done with message, deleting key from bucket, %v\n", v.ID)
				r.metrics.promMessagesProcessedTotal.Set(float64(v.ID))
				// case <-time.After(time.Second * 3):
				// 	// Testing with a timeout here to figure out if messages are stuck
				// 	// waiting for done signal.
				// 	fmt.Printf(" *** Ingo: message %v seems to be stuck, dropping message\n", v.ID)
			}

			// Since we are now done with the specific message we can delete
			// it out of the K/V Store.
			r.deleteKeyFromBucket(samValueBucket, strconv.Itoa(v.ID))

			r.permStore <- fmt.Sprintf("%v : %+v\n", time.Now().UTC(), v)

			// TODO: REMOVE: Dump the whole KV store
			// err := r.printBucketContent(samValueBucket)
			// if err != nil {
			// 	log.Printf("* Error: dump of db failed: %v\n", err)
			// }
		}(v)

	}

	close(outCh)
}

// dumpBucket will dump out all they keys and values in the
// specified bucket, and return a sorted []samDBValue
func (r *ringBuffer) dumpBucket(bucket string) ([]samDBValue, error) {
	samDBValues := []samDBValue{}

	err := r.db.View(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(bucket))
		if bu == nil {
			return fmt.Errorf("error: dumpBucket: tx.bucket returned nil")
		}

		// For each element found in the DB, unmarshal, and put on slice.
		bu.ForEach(func(k, v []byte) error {
			var vv samDBValue
			err := json.Unmarshal(v, &vv)
			if err != nil {
				log.Printf("error: dumpBucket json.Umarshal failed: %v\n", err)
			}
			samDBValues = append(samDBValues, vv)
			return nil
		})

		// Sort the order of the slice items based on ID, since they where retreived from a map.
		sort.SliceStable(samDBValues, func(i, j int) bool {
			return samDBValues[i].ID > samDBValues[j].ID
		})

		for _, v := range samDBValues {
			log.Printf("info: k/v store: %#v\n", v)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return samDBValues, err
}

// printBucketContent will print out all they keys and values in the
// specified bucket.
func (r *ringBuffer) printBucketContent(bucket string) error {
	err := r.db.View(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(bucket))

		bu.ForEach(func(k, v []byte) error {
			var vv samDBValue
			err := json.Unmarshal(v, &vv)
			if err != nil {
				log.Printf("error: printBucketContent json.Umarshal failed: %v\n", err)
			}
			log.Printf("k: %s, v: %v\n", k, vv)
			return nil
		})

		return nil
	})

	return err
}

// deleteKeyFromBucket will delete the specified key from the specified
// bucket if it exists.
func (r *ringBuffer) deleteKeyFromBucket(bucket string, key string) error {
	err := r.db.Update(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(bucket))

		err := bu.Delete([]byte(key))
		if err != nil {
			log.Printf("error: delete key in bucket %v failed: %v\n", bucket, err)
		}

		return nil
	})

	return err
}

// getIndexValue will get the last index value stored in DB.
func (r *ringBuffer) getIndexValue(indexBucket string) int {
	const indexKey string = "index"
	indexB, err := r.dbView(r.db, indexBucket, indexKey)
	if err != nil {
		log.Printf("error: getIndexValue: dbView: %v\n", err)
	}

	index, err := strconv.Atoi(string(indexB))
	if err != nil && string(indexB) == "" {
		log.Printf("info: getIndexValue: no index value found, probaly empty database, and no previous entries in db to process : %v\n", err)
	}

	// fmt.Printf("\n**** ringBuffer.getIndexValue: got index value = %v\n\n", index)

	return index
}

// dbView will look up and return a specific value if it exists for a key in a bucket in a DB.
func (r *ringBuffer) dbView(db *bolt.DB, bucket string, key string) ([]byte, error) {
	var value []byte
	// View is a help function to get values out of the database.
	err := db.View(func(tx *bolt.Tx) error {
		//Open a bucket to get key's and values from.
		bu := tx.Bucket([]byte(bucket))
		if bu == nil {
			log.Printf("info: no db bucket exist: %v\n", bucket)
			return nil
		}

		v := bu.Get([]byte(key))
		if len(v) == 0 {
			log.Printf("info: view: key not found\n")
			return nil
		}

		value = v

		return nil
	})

	return value, err

}

//dbUpdate will update the specified bucket with a key and value.
func (r *ringBuffer) dbUpdate(db *bolt.DB, bucket string, key string, value []byte) error {
	err := db.Update(func(tx *bolt.Tx) error {
		//Create a bucket
		bu, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return fmt.Errorf("error: CreateBuckerIfNotExists failed: %v", err)
		}

		//Put a value into the bucket.
		if err := bu.Put([]byte(key), []byte(value)); err != nil {
			return err
		}

		//If all was ok, we should return a nil for a commit to happen. Any error
		// returned will do a rollback.
		return nil
	})
	return err
}

// startPermStore will start the process that will handle writing of
// handled message to a permanent file.
// To store a message in the store, send what to store on the
// ringbuffer.permStore channel.
func (r *ringBuffer) startPermanentStore() {
	const storeFile string = "store.log"
	f, err := os.OpenFile(storeFile, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		log.Printf("error: startPermanentStore: failed to open file: %v\n", err)
	}
	defer f.Close()

	for {
		d := <-r.permStore
		_, err := f.WriteString(d)
		if err != nil {
			log.Printf("error:failed to write entry: %v\n", err)
		}
	}

}
