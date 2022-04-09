package steward

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	bolt "go.etcd.io/bbolt"
)

type signatureBase32 string
type argsString string
type centralAuth struct {
	// schema           map[Node]map[argsString]signatureBase32
	nodePublicKeys       *nodePublicKeys
	configuration        *Configuration
	db                   *bolt.DB
	bucketNamePublicKeys string
	errorKernel          *errorKernel
}

func newCentralAuth(configuration *Configuration, errorKernel *errorKernel) *centralAuth {
	c := centralAuth{
		// schema:           make(map[Node]map[argsString]signatureBase32),
		nodePublicKeys:       newNodePublicKeys(configuration),
		configuration:        configuration,
		bucketNamePublicKeys: "publicKeys",
		errorKernel:          errorKernel,
	}

	databaseFilepath := filepath.Join(configuration.DatabaseFolder, "auth.db")

	// Open the database file for persistent storage of public keys.
	db, err := bolt.Open(databaseFilepath, 0600, nil)
	if err != nil {
		log.Printf("error: failed to open db: %v\n", err)
		os.Exit(1)
	}

	c.db = db

	// Get public keys from db storage.
	keys, err := c.dbDumpPublicKey()
	if err != nil {
		log.Printf("debug: dbPublicKeyDump failed, probably empty db: %v\n", err)
	}

	// Only assign from storage to in memory map if the storage contained any values.
	if keys != nil {
		c.nodePublicKeys.KeyMap = keys
		for k, v := range keys {
			log.Printf("info: public keys db contains: %v, %v\n", k, []byte(v))
		}
	}

	return &c
}

// addPublicKey to the db if the node do not exist, or if it is a new value.
// We should return an error if the key have changed.
func (c *centralAuth) addPublicKey(proc process, msg Message) {
	c.nodePublicKeys.mu.Lock()

	// Check if a key for the current node already exists in the map.
	existingKey, ok := c.nodePublicKeys.KeyMap[msg.FromNode]

	if ok && bytes.Equal(existingKey, msg.Data) {
		fmt.Printf(" * key value for node %v is the same, doing nothing\n", msg.FromNode)
		c.nodePublicKeys.mu.Unlock()
		return
	}

	// New key
	c.nodePublicKeys.KeyMap[msg.FromNode] = msg.Data
	c.nodePublicKeys.mu.Unlock()

	// Add key to persistent storage.
	c.dbUpdatePublicKey(string(msg.FromNode), msg.Data)

	if ok {
		er := fmt.Errorf("info: updated with new public key for node: %v", msg.FromNode)
		fmt.Printf(" * %v\n", er)
		c.errorKernel.infoSend(proc, msg, er)
	}
	if !ok {
		er := fmt.Errorf("info: added public key for new node: %v", msg.FromNode)
		fmt.Printf(" * %v\n", er)
		c.errorKernel.infoSend(proc, msg, er)
	}

	//c.dbDump(c.bucketPublicKeys)
}

// dbView will look up and return a specific value if it exists for a key in a bucket in a DB.
func (c *centralAuth) dbGetPublicKey(node string) ([]byte, error) {
	var value []byte
	// View is a help function to get values out of the database.
	err := c.db.View(func(tx *bolt.Tx) error {
		//Open a bucket to get key's and values from.
		bu := tx.Bucket([]byte(c.bucketNamePublicKeys))
		if bu == nil {
			log.Printf("info: no db bucket exist: %v\n", c.bucketNamePublicKeys)
			return nil
		}

		v := bu.Get([]byte(node))
		if len(v) == 0 {
			log.Printf("info: view: key not found\n")
			return nil
		}

		value = v

		return nil
	})

	return value, err
}

//dbUpdatePublicKey will update the public key for a node in the db.
func (c *centralAuth) dbUpdatePublicKey(node string, value []byte) error {
	err := c.db.Update(func(tx *bolt.Tx) error {
		//Create a bucket
		bu, err := tx.CreateBucketIfNotExists([]byte(c.bucketNamePublicKeys))
		if err != nil {
			return fmt.Errorf("error: CreateBuckerIfNotExists failed: %v", err)
		}

		//Put a value into the bucket.
		if err := bu.Put([]byte(node), []byte(value)); err != nil {
			return err
		}

		//If all was ok, we should return a nil for a commit to happen. Any error
		// returned will do a rollback.
		return nil
	})
	return err
}

// deleteKeyFromBucket will delete the specified key from the specified
// bucket if it exists.
func (c *centralAuth) dbDeletePublicKey(key string) error {
	err := c.db.Update(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(c.bucketNamePublicKeys))

		err := bu.Delete([]byte(key))
		if err != nil {
			log.Printf("error: delete key in bucket %v failed: %v\n", c.bucketNamePublicKeys, err)
		}

		return nil
	})

	return err
}

// dumpBucket will dump out all they keys and values in the
// specified bucket, and return a sorted []samDBValue
func (c *centralAuth) dbDumpPublicKey() (map[Node][]byte, error) {
	m := make(map[Node][]byte)

	err := c.db.View(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(c.bucketNamePublicKeys))
		if bu == nil {
			return fmt.Errorf("error: dumpBucket: tx.bucket returned nil")
		}

		// For each element found in the DB, print it.
		bu.ForEach(func(k, v []byte) error {
			m[Node(k)] = v
			return nil
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return m, nil
}

// nodePublicKeys holds all the gathered public keys of nodes in the system.
// The keys will be written to a k/v store for persistence.
type nodePublicKeys struct {
	mu     sync.RWMutex
	KeyMap map[Node][]byte
}

// newNodePublicKeys will return a prepared type of nodePublicKeys.
func newNodePublicKeys(configuration *Configuration) *nodePublicKeys {
	n := nodePublicKeys{
		KeyMap: make(map[Node][]byte),
	}

	return &n
}
