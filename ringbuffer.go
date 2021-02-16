// Info: The idea about the ring buffer is that we have a FIFO
// buffer where we store all incomming messages requested by
// operators. Each message processed will also be stored in a DB.
//
// Idea: All incomming messages should be handled from the in-memory
// buffered channel, but when they are put on the buffer they should
// also be written to the DB with a handled flag set to false.
// When a message have left the buffer the handled flag should be
// set to true.
package steward

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

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
	bufData            chan subjectAndMessage
	db                 *bolt.DB
	totalMessagesIndex int
}

// newringBuffer is a push/pop storage for values.
func newringBuffer(size int) *ringBuffer {
	db, err := bolt.Open("./incommmingBuffer.db", 0600, nil)
	if err != nil {
		log.Printf("error: failed to open db: %v\n", err)
	}
	return &ringBuffer{
		bufData: make(chan subjectAndMessage, size),
		db:      db,
	}
}

// start will process incomming messages through the inCh,
// put the messages on a buffered channel
// and deliver messages out when requested on the outCh.
func (r *ringBuffer) start(inCh chan subjectAndMessage, outCh chan subjectAndMessage) {
	// Starting both writing and reading in separate go routines so we
	// can write and read concurrently.

	const samValueBucket string = "samValueBucket"
	const indexValueBucket string = "indexValueBucket"

	r.totalMessagesIndex = r.getIndexValue(indexValueBucket)

	// Fill the buffer when new data arrives
	go func() {
		// Check for incomming messages. These are typically comming from
		// the go routine who reads inmsg.txt.
		for v := range inCh {
			// --- Store the incomming message in the k/v store ---

			// Create a structure for JSON marshaling.
			samV := samDBValue{
				ID:   r.totalMessagesIndex,
				Data: v,
			}

			js, err := json.Marshal(samV)
			if err != nil {
				log.Printf("error: gob encoding samValue: %v\n", err)
			}

			// Store the incomming message in key/value store
			err = r.dbUpdate(r.db, samValueBucket, strconv.Itoa(r.totalMessagesIndex), js)
			if err != nil {
				// TODO: Handle error
				log.Printf("error: dbUpdate samValue failed: %v\n", err)
			}

			// Send the message to some process to consume it.
			r.bufData <- v

			// Increment index, and store the new value to the database.
			r.totalMessagesIndex++
			fmt.Printf("*** NEXT INDEX NUMBER INCREMENTED: %v\n", r.totalMessagesIndex)
			fmt.Println("---------------------------------------------------------")
			r.dbUpdate(r.db, indexValueBucket, "index", []byte(strconv.Itoa(r.totalMessagesIndex)))
		}

		// When done close the buffer channel
		close(r.bufData)
	}()

	// Empty the buffer when data asked for
	go func() {
		// Range over the buffer of messages to pass on to processes.
		for v := range r.bufData {
			// Create a done channel per message. A process started by the
			// spawnProcess function will handle incomming messages sequentaly.
			// So in the spawnProcess function we put a struct{} value when a
			// message is processed on the "done" channel and an ack is received
			// for a message, and we wait here for the "done" to be received.

			v.Message.done = make(chan struct{})
			outCh <- v

			// ----------TESTING
			<-v.done
			fmt.Println("-----------------------------------------------------------")
			fmt.Printf("### DONE WITH THE MESSAGE\n")
			fmt.Println("-----------------------------------------------------------")

			// TODO: Delete the messages here. The SAM handled here, do
			// not contain the totalMessageID, so we might need to change
			// the struct we pass around.
			// IDEA: Add a go routine for each message handled here, and include
			// a done channel in the structure, so a go routine handling the
			// message will be able to signal back here that the message have
			// been processed, and that we then can delete it out of the K/V Store.

			// Dump the whole KV store
			err := r.dumpBucket(samValueBucket)
			if err != nil {
				fmt.Printf("* Error: dump of db failed: %v\n", err)
			}

		}

		close(outCh)
	}()
}

func (r *ringBuffer) dumpBucket(bucket string) error {
	err := r.db.View(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(bucket))

		fmt.Println("--------------------------DUMP---------------------------")
		bu.ForEach(func(k, v []byte) error {
			var vv samDBValue
			err := json.Unmarshal(v, &vv)
			if err != nil {
				log.Printf("error: dumpBucket json.Umarshal failed: %v\n", err)
			}
			fmt.Printf("k: %s, v: %v\n", k, vv)
			return nil
		})

		// c := bu.Cursor()
		//
		// fmt.Println("--------------------------DUMP---------------------------")
		// for k, v := c.First(); k != nil; c.Next() {
		// 	fmt.Printf("k: %s, v: %v\n", k, v)
		// }

		return nil
	})

	return err
}

func (r *ringBuffer) getIndexValue(indexBucket string) int {
	const indexKey string = "index"
	indexB, err := r.dbView(r.db, indexBucket, indexKey)
	if err != nil {
		log.Printf("error: getIndexValue: dbView: %v\n", err)
	}

	index, err := strconv.Atoi(string(indexB))
	if err != nil {
		log.Printf("error: getIndexValue: strconv.Atoi : %v\n", err)
	}

	fmt.Printf("**** RETURNING INDEX, WITH VALUE = %v\n", index)

	return index
}

func (r *ringBuffer) dbView(db *bolt.DB, bucket string, key string) ([]byte, error) {
	var value []byte
	//View is a help function to get values out of the database.
	err := db.View(func(tx *bolt.Tx) error {
		//Open a bucket to get key's and values from.
		bu := tx.Bucket([]byte(bucket))
		if bu == nil {
			log.Printf("info: no such bucket exist: %v\n", bucket)
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
