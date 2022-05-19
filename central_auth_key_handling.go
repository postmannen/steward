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

// centralAuth holds the logic related to handling public keys and auth maps.
type centralAuth struct {
	// acl and authorization level related data and methods.
	accessLists *accessLists
	// public key distribution related data and methods.
	pki *pki
}

// newCentralAuth will return a new and prepared *centralAuth
func newCentralAuth(configuration *Configuration, errorKernel *errorKernel) *centralAuth {
	c := centralAuth{
		accessLists: newAccessLists(errorKernel, configuration),
		pki:         newPKI(configuration, errorKernel),
	}

	return &c
}

// nodesAcked is the structure that holds all the keys that we have
// acknowledged, and that are allowed to be distributed within the
// system. It also contains a hash of all those keys.
type nodesAcked struct {
	mu          sync.Mutex
	keysAndHash *keysAndHash
}

// newNodesAcked will return a prepared *nodesAcked structure.
func newNodesAcked() *nodesAcked {
	n := nodesAcked{
		keysAndHash: newKeysAndHash(),
	}

	return &n
}

// pki holds the data and method relevant to key handling and distribution.
type pki struct {
	nodesAcked             *nodesAcked
	nodeNotAckedPublicKeys *nodeNotAckedPublicKeys
	configuration          *Configuration
	db                     *bolt.DB
	bucketNamePublicKeys   string
	errorKernel            *errorKernel
}

// newKeys will return a prepared *keys with input values set.
func newPKI(configuration *Configuration, errorKernel *errorKernel) *pki {
	p := pki{
		// schema:           make(map[Node]map[argsString]signatureBase32),
		nodesAcked:             newNodesAcked(),
		nodeNotAckedPublicKeys: newNodeNotAckedPublicKeys(configuration),
		configuration:          configuration,
		bucketNamePublicKeys:   "publicKeys",
		errorKernel:            errorKernel,
	}

	databaseFilepath := filepath.Join(configuration.DatabaseFolder, "auth.db")

	// Open the database file for persistent storage of public keys.
	db, err := bolt.Open(databaseFilepath, 0600, nil)
	if err != nil {
		log.Printf("error: failed to open db: %v\n", err)
		os.Exit(1)
	}

	p.db = db

	// Get public keys from db storage.
	keys, err := p.dbDumpPublicKey()
	if err != nil {
		log.Printf("debug: dbPublicKeyDump failed, probably empty db: %v\n", err)
	}

	// Only assign from storage to in memory map if the storage contained any values.
	if keys != nil {
		p.nodesAcked.keysAndHash.Keys = keys
		for k, v := range keys {
			log.Printf("info: public keys db contains: %v, %v\n", k, []byte(v))
		}
	}

	// Get the current hash from db if one exists.
	hash, err := p.dbViewHash()
	if err != nil {
		log.Printf("debug: dbViewHash failed: %v\n", err)
	}

	if hash != nil {
		var h [32]byte
		copy(h[:], hash)
		p.nodesAcked.keysAndHash.Hash = h
	}

	return &p
}

// addPublicKey to the db if the node do not exist, or if it is a new value.
func (p *pki) addPublicKey(proc process, msg Message) {

	// TODO: When receiviving a new or different keys for a node we should
	// have a service with it's own storage for these keys, and an operator
	// should have to acknowledge the new keys.
	// For this we need:
	//	- A service that keeps the state of all the new keys detected in the
	//	  bytes.equal check below.
	//	- A Log message should be thrown so we know that there is a new key.
	//	- A Request method that can be used by operator to acknowledge a new
	//	  key for a host.

	// Check if a key for the current node already exists in the map.
	p.nodesAcked.mu.Lock()
	existingKey, ok := p.nodesAcked.keysAndHash.Keys[msg.FromNode]
	p.nodesAcked.mu.Unlock()

	if ok && bytes.Equal(existingKey, msg.Data) {
		fmt.Printf(" \n * public key value for REGISTERED node %v is the same, doing nothing\n\n", msg.FromNode)
		return
	}

	p.nodeNotAckedPublicKeys.mu.Lock()
	existingNotAckedKey, ok := p.nodeNotAckedPublicKeys.KeyMap[msg.FromNode]
	// We only want to send one notification to the error kernel about new key detection,
	// so we check if the values are the same as the one we already got before we continue
	// with registering and logging for the the new key.
	if ok && bytes.Equal(existingNotAckedKey, msg.Data) {
		fmt.Printf(" * \nkey value for NOT-REGISTERED node %v is the same, doing nothing\n\n", msg.FromNode)
		p.nodeNotAckedPublicKeys.mu.Unlock()
		return
	}

	p.nodeNotAckedPublicKeys.KeyMap[msg.FromNode] = msg.Data
	p.nodeNotAckedPublicKeys.mu.Unlock()

	er := fmt.Errorf("info: detected new public key for node: %v. This key will need to be authorized by operator to be allowed into the system", msg.FromNode)
	fmt.Printf(" * %v\n", er)
	p.errorKernel.infoSend(proc, msg, er)
}

// // dbGetPublicKey will look up and return a specific value if it exists for a key in a bucket in a DB.
// func (c *centralAuth) dbGetPublicKey(node string) ([]byte, error) {
// 	var value []byte
// 	// View is a help function to get values out of the database.
// 	err := c.db.View(func(tx *bolt.Tx) error {
// 		//Open a bucket to get key's and values from.
// 		bu := tx.Bucket([]byte(c.bucketNamePublicKeys))
// 		if bu == nil {
// 			log.Printf("info: no db bucket exist: %v\n", c.bucketNamePublicKeys)
// 			return nil
// 		}
//
// 		v := bu.Get([]byte(node))
// 		if len(v) == 0 {
// 			log.Printf("info: view: key not found\n")
// 			return nil
// 		}
//
// 		value = v
//
// 		return nil
// 	})
//
// 	return value, err
// }

//dbUpdatePublicKey will update the public key for a node in the db.
func (p *pki) dbUpdatePublicKey(node string, value []byte) error {
	err := p.db.Update(func(tx *bolt.Tx) error {
		//Create a bucket
		bu, err := tx.CreateBucketIfNotExists([]byte(p.bucketNamePublicKeys))
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

//dbUpdateHash will update the public key for a node in the db.
func (p *pki) dbUpdateHash(hash []byte) error {
	err := p.db.Update(func(tx *bolt.Tx) error {
		//Create a bucket
		bu, err := tx.CreateBucketIfNotExists([]byte("hash"))
		if err != nil {
			return fmt.Errorf("error: CreateBuckerIfNotExists failed: %v", err)
		}

		//Put a value into the bucket.
		if err := bu.Put([]byte("hash"), []byte(hash)); err != nil {
			return err
		}

		//If all was ok, we should return a nil for a commit to happen. Any error
		// returned will do a rollback.
		return nil
	})
	return err
}

// dbViewHash will look up and return a specific value if it exists for a key in a bucket in a DB.
func (p *pki) dbViewHash() ([]byte, error) {
	var value []byte
	// View is a help function to get values out of the database.
	err := p.db.View(func(tx *bolt.Tx) error {
		//Open a bucket to get key's and values from.
		bu := tx.Bucket([]byte("hash"))
		if bu == nil {
			log.Printf("info: no db hash bucket exist\n")
			return nil
		}

		v := bu.Get([]byte("hash"))
		if len(v) == 0 {
			log.Printf("info: view: hash key not found\n")
			return nil
		}

		value = v

		return nil
	})

	return value, err

}

// // deleteKeyFromBucket will delete the specified key from the specified
// // bucket if it exists.
// func (c *centralAuth) dbDeletePublicKey(key string) error {
// 	err := c.db.Update(func(tx *bolt.Tx) error {
// 		bu := tx.Bucket([]byte(c.bucketNamePublicKeys))
//
// 		err := bu.Delete([]byte(key))
// 		if err != nil {
// 			log.Printf("error: delete key in bucket %v failed: %v\n", c.bucketNamePublicKeys, err)
// 		}
//
// 		return nil
// 	})
//
// 	return err
// }

// dumpBucket will dump out all they keys and values in the
// specified bucket, and return a sorted []samDBValue
func (p *pki) dbDumpPublicKey() (map[Node][]byte, error) {
	m := make(map[Node][]byte)

	err := p.db.View(func(tx *bolt.Tx) error {
		bu := tx.Bucket([]byte(p.bucketNamePublicKeys))
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

// --- HERE

// nodeNotAckedPublicKeys holds all the gathered but not acknowledged public
// keys of nodes in the system.
type nodeNotAckedPublicKeys struct {
	mu     sync.RWMutex
	KeyMap map[Node][]byte
}

// newNodeNotAckedPublicKeys will return a prepared type of nodePublicKeys.
func newNodeNotAckedPublicKeys(configuration *Configuration) *nodeNotAckedPublicKeys {
	n := nodeNotAckedPublicKeys{
		KeyMap: make(map[Node][]byte),
	}

	return &n
}
