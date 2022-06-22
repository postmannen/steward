// Info: The idea about the ring buffer is that we have a FIFO
// buffer where we store all incomming messages requested by
// operators.
// Each message in process or waiting to be processed will be
// stored in a DB. When the processing of a given message is
// done it will be removed from the state db, and an entry will
// made in the persistent message log.

package steward

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	copier "github.com/jinzhu/copier"
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
	db               *bolt.DB
	samValueBucket   string
	indexValueBucket string
	// The current number of items in the database.
	totalMessagesIndex int
	mu                 sync.Mutex
	// The channel to send messages that have been processed,
	// and we want to store it in the permanent message log.
	permStore chan string
	// Name of node.
	nodeName Node
	// ringBufferBulkInCh from *server are also implemented here,
	// so the ringbuffer can send it's error messages the same
	// way as all messages are handled.
	ringBufferBulkInCh chan []subjectAndMessage
	metrics            *metrics
	configuration      *Configuration
	errorKernel        *errorKernel
	processInitial     process
}

// newringBuffer returns a push/pop storage for values.
func newringBuffer(ctx context.Context, metrics *metrics, configuration *Configuration, size int, dbFileName string, nodeName Node, ringBufferBulkInCh chan []subjectAndMessage, samValueBucket string, indexValueBucket string, errorKernel *errorKernel, processInitial process) *ringBuffer {

	r := ringBuffer{}

	// Check if socket folder exists, if not create it
	if _, err := os.Stat(configuration.DatabaseFolder); os.IsNotExist(err) {
		err := os.MkdirAll(configuration.DatabaseFolder, 0700)
		if err != nil {
			log.Printf("error: failed to create database directory %v: %v\n", configuration.DatabaseFolder, err)
			os.Exit(1)
		}
	}

	DatabaseFilepath := filepath.Join(configuration.DatabaseFolder, dbFileName)

	// ---
	var db *bolt.DB
	if configuration.RingBufferPersistStore {
		var err error
		db, err = bolt.Open(DatabaseFilepath, 0600, nil)
		if err != nil {
			log.Printf("error: failed to open db: %v\n", err)
			os.Exit(1)
		}
	}

	r.bufData = make(chan samDBValue, size)
	r.db = db
	r.samValueBucket = samValueBucket
	r.indexValueBucket = indexValueBucket
	r.permStore = make(chan string)
	r.nodeName = nodeName
	r.ringBufferBulkInCh = ringBufferBulkInCh
	r.metrics = metrics
	r.configuration = configuration
	r.processInitial = processInitial

	return &r
}

// start will process incomming messages through the inCh,
// put the messages on a buffered channel
// and deliver messages out when requested on the outCh.
func (r *ringBuffer) start(ctx context.Context, inCh chan subjectAndMessage, outCh chan samDBValueAndDelivered) {

	// Starting both writing and reading in separate go routines so we
	// can write and read concurrently.

	r.totalMessagesIndex = 0
	if r.configuration.RingBufferPersistStore {
		r.totalMessagesIndex = r.getIndexValue()
	}

	// Fill the buffer when new data arrives into the system
	go r.fillBuffer(ctx, inCh)

	// Start the process to permanently store done messages.
	go r.startPermanentStore(ctx)

	// Start the process that will handle messages present in the ringbuffer.
	go r.processBufferMessages(ctx, outCh)

	go func() {
		ticker := time.NewTicker(time.Second * 5)

		for {
			select {
			case <-ticker.C:
				if r.configuration.RingBufferPersistStore {
					r.dbUpdateMetrics(r.samValueBucket)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// fillBuffer will fill the buffer in the ringbuffer  reading from the inchannel.
// It will also store the messages in a K/V DB while being processed.
func (r *ringBuffer) fillBuffer(ctx context.Context, inCh chan subjectAndMessage) {
	// At startup get all the values that might be in the K/V store so we can
	// put them into the buffer before we start to fill up with new incomming
	// messages to the system.
	// This is needed when the program have been restarted, and we need to check
	// if there where previously unhandled messages that need to be handled first.

	if r.configuration.RingBufferPersistStore {
		func() {
			s, err := r.dumpBucket(r.samValueBucket)
			if err != nil {
				er := fmt.Errorf("info: fillBuffer: retreival of values from k/v store failed, probaly empty database, and no previous entries in db to process: %v", err)
				log.Printf("%v\n", er)
				return
			}

			for _, v := range s {
				r.bufData <- v
			}
		}()
	}

	// Prepare the map structure to know what values are allowed
	// for the events
	var event Event
	eventAvailable := event.CheckEventAvailable()
	eventAvailableValues := []Event{}
	for v := range eventAvailable.topics {
		eventAvailableValues = append(eventAvailableValues, v)
	}

	// Check for incomming messages. These are typically comming from
	// the go routine who reads the socket.
	for {
		select {
		case v := <-inCh:
			// Check if the event exists.
			if !eventAvailable.CheckIfExists(v.Event, v.Subject) {
				er := fmt.Errorf("error: fillBuffer: the event type do not exist, so this message will not be put on the buffer to be processed. Check the syntax used in the json file for the message. Allowed values are : %v, where given: event=%v, with subject=%v", eventAvailableValues, v.Event, v.Subject)
				r.errorKernel.errSend(r.processInitial, Message{}, er)

				// if it was not a valid value, we jump back up, and
				// continue the range iteration.
				continue
			}

			// Check if default message values for timers are set, and if
			// not then set default message values.
			if v.Message.ACKTimeout < 1 {
				v.Message.ACKTimeout = r.configuration.DefaultMessageTimeout
			}
			if v.Message.Retries < 1 {
				v.Message.Retries = r.configuration.DefaultMessageRetries
			}
			if v.Message.MethodTimeout < 1 && v.Message.MethodTimeout != -1 {
				v.Message.MethodTimeout = r.configuration.DefaultMethodTimeout
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

			if r.configuration.RingBufferPersistStore {
				js, err := json.Marshal(samV)
				if err != nil {
					er := fmt.Errorf("error:fillBuffer: json marshaling: %v", err)
					r.errorKernel.errSend(r.processInitial, Message{}, er)
				}

				// Store the incomming message in key/value store
				err = r.dbUpdate(r.db, r.samValueBucket, strconv.Itoa(dbID), js)
				if err != nil {
					er := fmt.Errorf("error: dbUpdate samValue failed: %v", err)
					r.errorKernel.errSend(r.processInitial, Message{}, er)
				}
			}

			// Put the message on the inmemory buffer.
			r.bufData <- samV

			// Increment index, and store the new value to the database.
			r.mu.Lock()
			r.totalMessagesIndex++
			if r.configuration.RingBufferPersistStore {
				r.dbUpdate(r.db, r.indexValueBucket, "index", []byte(strconv.Itoa(r.totalMessagesIndex)))
			}
			r.mu.Unlock()
		case <-ctx.Done():
			// When done close the buffer channel
			close(r.bufData)
			return
		}

	}
}

// processBufferMessages will pick messages from the buffer, and process them
// one by one. The messages will be delivered on the outCh, and it will wait
// until a signal is received on the done channel before it continues with the
// next message.
func (r *ringBuffer) processBufferMessages(ctx context.Context, outCh chan samDBValueAndDelivered) {
	// Range over the buffer of messages to pass on to processes.
	for {
		select {
		case v := <-r.bufData:
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
				// Create a copy of the message that we can use to write to the
				// perm store without causing a race since the REQ handler for the
				// message might not yet be done when message is written to the
				// perm store.
				// We also need a copy to be able to remove the data from the message
				// when writing it to the store, so we don't mess up to actual data
				// that might be in use in the handler.

				msgForPermStore := Message{}
				copier.Copy(&msgForPermStore, v.Data.Message)
				// Remove the content of the data field.
				msgForPermStore.Data = nil

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
					// TODO: Check if more logic should be made here if messages are stuck etc.
					// Testing with a timeout here to figure out if messages are stuck
					// waiting for done signal.
					log.Printf("Error: *** message %v seems to be stuck, did not receive delivered signal from reading process\n", v.ID)

					r.metrics.promRingbufferStalledMessagesTotal.Inc()
				}
				// Listen on the done channel here , so a go routine handling the
				// message will be able to signal back here that the message have
				// been processed, and that we then can delete it out of the K/V Store.

				<-v.Data.done
				// log.Printf("info: processBufferMessages: done with message, deleting key from bucket, %v\n", v.ID)
				r.metrics.promMessagesProcessedIDLast.Set(float64(v.ID))

				// Since we are now done with the specific message we can delete
				// it out of the K/V Store.
				if r.configuration.RingBufferPersistStore {
					r.deleteKeyFromBucket(r.samValueBucket, strconv.Itoa(v.ID))
				}

				js, err := json.Marshal(msgForPermStore)
				if err != nil {
					er := fmt.Errorf("error:fillBuffer: json marshaling: %v", err)
					r.errorKernel.errSend(r.processInitial, Message{}, er)
				}
				r.permStore <- time.Now().Format("Mon Jan _2 15:04:05 2006") + ", " + string(js) + "\n"

			}(v)
		case <-ctx.Done():
			//close(outCh)
			return
		}
	}
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
			log.Printf("info: k/v store, kvID: %v, message.ID: %v, subject: %v, len(data): %v\n", v.ID, v.Data.ID, v.Data.Subject, len(v.Data.Data))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return samDBValues, err
}

// // printBucketContent will print out all they keys and values in the
// // specified bucket.
// func (r *ringBuffer) printBucketContent(bucket string) error {
// 	err := r.db.View(func(tx *bolt.Tx) error {
// 		bu := tx.Bucket([]byte(bucket))
//
// 		bu.ForEach(func(k, v []byte) error {
// 			var vv samDBValue
// 			err := json.Unmarshal(v, &vv)
// 			if err != nil {
// 				log.Printf("error: printBucketContent json.Umarshal failed: %v\n", err)
// 			}
// 			log.Printf("k: %s, v: %v\n", k, vv)
// 			return nil
// 		})
//
// 		return nil
// 	})
//
// 	return err
// }

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

// db update metrics.
func (r *ringBuffer) dbUpdateMetrics(bucket string) error {
	err := r.db.Update(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(bucket))

		r.metrics.promDBMessagesCurrent.Set(float64(bu.Stats().KeyN))

		return nil
	})

	return err
}

// getIndexValue will get the last index value stored in DB.
func (r *ringBuffer) getIndexValue() int {
	const indexKey string = "index"
	indexB, err := r.dbView(r.db, r.indexValueBucket, indexKey)
	if err != nil {
		log.Printf("error: getIndexValue: dbView: %v\n", err)
	}

	index, err := strconv.Atoi(string(indexB))
	if err != nil && string(indexB) == "" {
		log.Printf("info: getIndexValue: no index value found, probaly empty database, and no previous entries in db to process : %v\n", err)
	}

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
func (r *ringBuffer) startPermanentStore(ctx context.Context) {
	const storeFile string = "store.log"
	f, err := os.OpenFile(storeFile, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		log.Printf("error: startPermanentStore: failed to open file: %v\n", err)
	}
	defer f.Close()

	for {
		select {
		case d := <-r.permStore:
			_, err := f.WriteString(d)
			if err != nil {
				log.Printf("error:failed to write entry: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}

}
