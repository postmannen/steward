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
	"bytes"
	"encoding/gob"
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
	bufData chan subjectAndMessage
	db      *bolt.DB
}

// newringBuffer is a push/pop storage for values.
func newringBuffer(size int) *ringBuffer {
	db, err := bolt.Open("./incommmingBuffer.db", 0600, nil)
	if err != nil {
		// TODO: error handling
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

	const samValueBucket string = "samValues"

	i := 0

	// Fill the buffer when new data arrives
	go func() {
		for v := range inCh {
			r.bufData <- v
			fmt.Printf("**BUFFER** DEBUG PUSHED ON BUFFER: value = %v\n\n", v)

			iv := strconv.Itoa(i)
			samV := samDBValue{
				ID:   i,
				Data: v,
			}

			svGob, err := gobEncodeSamValue(samV)
			if err != nil {
				log.Printf("error: gob encoding samValue: %v\n", err)
			}

			// Also store the incomming message in key/value store
			err = r.dbUpdate(r.db, samValueBucket, iv, svGob)
			if err != nil {
				// TODO: Handle error
				log.Printf("error: dbUpdate samValue failed: %v\n", err)
			}

			retreivedGob, err := r.dbView(r.db, samValueBucket, iv)
			if err != nil {
				// TODO: Handle error
				log.Printf("error: dbView retreival samValue failed: %v\n", err)
			}

			retreived, err := gobDecodeSamValue(retreivedGob)
			if err != nil {
				// TODO: Handle error
				log.Printf("error: dbView gobDecode retreival samValue failed: %v\n", err)
			}

			fmt.Printf("*************** INFO: dbView, key: %v, got value: %v\n ", iv, retreived)

			i++
		}

		// When done close the buffer channel
		close(r.bufData)
	}()

	// Empty the buffer when data asked for
	go func() {
		for v := range r.bufData {
			outCh <- v
		}

		close(outCh)
	}()
}

func (r *ringBuffer) dbView(db *bolt.DB, bucket string, key string) ([]byte, error) {
	var value []byte
	//View is a help function to get values out of the database.
	err := db.View(func(tx *bolt.Tx) error {
		//Open a bucket to get key's and values from.
		bu := tx.Bucket([]byte(bucket))

		v := bu.Get([]byte(key))
		if len(v) == 0 {
			return fmt.Errorf("info: view: key not found")
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

func gobEncodeSamValue(samValue samDBValue) ([]byte, error) {
	var buf bytes.Buffer
	gobEnc := gob.NewEncoder(&buf)
	err := gobEnc.Encode(samValue)
	if err != nil {
		return nil, fmt.Errorf("error: gob.Encode failed: %v", err)
	}

	return buf.Bytes(), nil
}

func gobDecodeSamValue(b []byte) (samDBValue, error) {
	sv := samDBValue{}

	buf := bytes.NewBuffer(b)
	gobDec := gob.NewDecoder(buf)
	err := gobDec.Decode(&sv)
	if err != nil {
		log.Printf("error: gob decoding failed: %v\n", err)
		return samDBValue{}, err
	}

	return sv, nil
}
