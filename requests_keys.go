package steward

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"github.com/fxamacker/cbor/v2"
)

// ---

type methodREQPublicKey struct {
	event Event
}

func (m methodREQPublicKey) getKind() Event {
	return m.event
}

// Handler to get the public ed25519 key from a node.
func (m methodREQPublicKey) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.
	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			case outCh <- proc.nodeAuth.SignPublicKey:
			}
		}()

		select {
		// case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-ctx.Done():
		case out := <-outCh:

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, out)
		}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQKeysRequestUpdate struct {
	event Event
}

func (m methodREQKeysRequestUpdate) getKind() Event {
	return m.event
}

// Handler to get all the public ed25519 keys from a central server.
func (m methodREQKeysRequestUpdate) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.

	// TODO:
	// - Since this is implemented as a NACK message we could implement a
	//   metric thats shows the last time a node did a key request.
	// - We could also implement a metrics on the receiver showing the last
	//   time a node had done an update.

	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			// TODO: Should we receive a hash of he current keys from the node here
			// to verify if we need to update or not ?
			case outCh <- []byte{}:
			}
		}()

		select {
		case <-ctx.Done():
		// case out := <-outCh:
		case <-outCh:
			// Using a func here to set the scope of the lock, and then be able to
			// defer the unlock when leaving that scope.
			func() {
				proc.centralAuth.pki.nodesAcked.mu.Lock()
				defer proc.centralAuth.pki.nodesAcked.mu.Unlock()
				// TODO: We should probably create a hash of the current map content,
				// store it alongside the KeyMap, and send both the KeyMap and hash
				// back. We can then later send that hash when asking for keys, compare
				// it with the current one for the KeyMap, and know if we need to send
				// and update back to the node who published the request to here.

				fmt.Printf(" <---- methodREQKeysRequestUpdate: received hash from NODE=%v, HASH=%v\n", message.FromNode, message.Data)

				// Check if the received hash is the same as the one currently active,
				if bytes.Equal(proc.centralAuth.pki.nodesAcked.keysAndHash.Hash[:], message.Data) {
					fmt.Printf("\n --- methodREQKeysRequestUpdate:  NODE AND CENTRAL HAVE EQUAL KEYS, NOTHING TO DO, EXITING HANDLER\n\n")
					return
				}

				fmt.Printf("\n ------------methodREQKeysRequestUpdate: NODE AND CENTRAL HAD NOT EQUAL KEYS, PREPARING TO SEND NEW VERSION OF KEYS\n\n")

				fmt.Printf(" *     methodREQKeysRequestUpdate: marshalling new keys and hash to send: map=%v, hash=%v\n\n", proc.centralAuth.pki.nodesAcked.keysAndHash.Keys, proc.centralAuth.pki.nodesAcked.keysAndHash.Hash)

				b, err := json.Marshal(proc.centralAuth.pki.nodesAcked.keysAndHash)

				if err != nil {
					er := fmt.Errorf("error: methodREQKeysRequestUpdate, failed to marshal keys map: %v", err)
					proc.errorKernel.errSend(proc, message, er)
				}
				fmt.Printf("\n ----> methodREQKeysRequestUpdate: SENDING KEYS TO NODE=%v\n", message.FromNode)
				newReplyMessage(proc, message, b)
			}()
		}
	}()

	// NB: We're not sending an ACK message for this request type.
	return nil, nil
}

// ----

type methodREQKeysDeliverUpdate struct {
	event Event
}

func (m methodREQKeysDeliverUpdate) getKind() Event {
	return m.event
}

// Handler to receive the public keys from a central server.
func (m methodREQKeysDeliverUpdate) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.

	// TODO:
	// - Since this is implemented as a NACK message we could implement a
	//   metric thats shows the last time keys were updated.

	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			// TODO: Should we receive a hash of he current keys from the node here ?
			case outCh <- []byte{}:
			}
		}()

		select {
		// case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-ctx.Done():
		case <-outCh:

			proc.nodeAuth.publicKeys.mu.Lock()

			err := json.Unmarshal(message.Data, proc.nodeAuth.publicKeys.keysAndHash)
			fmt.Printf("\n <---- REQKeysDeliverUpdate: after unmarshal, nodeAuth keysAndhash contains: %+v\n\n", proc.nodeAuth.publicKeys.keysAndHash)

			proc.nodeAuth.publicKeys.mu.Unlock()

			if err != nil {
				er := fmt.Errorf("error: REQKeysDeliverUpdate : json unmarshal failed: %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			// We need to also persist the hash on the receiving nodes. We can then load
			// that key upon startup.

			err = proc.nodeAuth.publicKeys.saveToFile()
			if err != nil {
				er := fmt.Errorf("error: REQKeysDeliverUpdate : save to file failed: %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			// newReplyMessage(proc, message, out)
		}
	}()

	// Send back an ACK message.
	// ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return nil, nil
}

// ----

// TODO: We should also add a request method methodREQPublicKeysRevoke

type methodREQKeysAllow struct {
	event Event
}

func (m methodREQKeysAllow) getKind() Event {
	return m.event
}

// Handler to allow new public keys into the database on central auth.
// Nodes will send the public key in the REQHello messages. When they
// are recived on the central server they will be put into a temp key
// map, and we need to acknowledge them before they are moved into the
// main key map, and then allowed to be sent out to other nodes.
func (m methodREQKeysAllow) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.
	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			case outCh <- []byte{}:
			}
		}()

		select {
		case <-ctx.Done():
		case <-outCh:
			proc.centralAuth.pki.nodeNotAckedPublicKeys.mu.Lock()
			defer proc.centralAuth.pki.nodeNotAckedPublicKeys.mu.Unlock()

			// Range over all the MethodArgs, where each element represents a node to allow,
			// and move the node from the notAcked map to the allowed map.
			for _, n := range message.MethodArgs {
				key, ok := proc.centralAuth.pki.nodeNotAckedPublicKeys.KeyMap[Node(n)]
				if ok {

					func() {
						proc.centralAuth.pki.nodesAcked.mu.Lock()
						defer proc.centralAuth.pki.nodesAcked.mu.Unlock()

						// Store/update the node and public key on the allowed pubKey map.
						proc.centralAuth.pki.nodesAcked.keysAndHash.Keys[Node(n)] = key
					}()

					// Add key to persistent storage.
					proc.centralAuth.pki.dbUpdatePublicKey(string(n), key)

					// Delete the key from the NotAcked map
					delete(proc.centralAuth.pki.nodeNotAckedPublicKeys.KeyMap, Node(n))

					er := fmt.Errorf("info: REQKeysAllow : allowed new/updated public key for %v to allowed public key map", n)
					proc.errorKernel.infoSend(proc, message, er)
				}
			}

			// All new elements are now added, and we can create a new hash
			// representing the current keys in the allowed map.
			func() {
				proc.centralAuth.pki.nodesAcked.mu.Lock()
				defer proc.centralAuth.pki.nodesAcked.mu.Unlock()

				type NodesAndKeys struct {
					Node Node
					Key  []byte
				}

				// Create a slice of all the map keys, and its value.
				sortedNodesAndKeys := []NodesAndKeys{}

				// Range the map, and add each k/v to the sorted slice, to be sorted later.
				for k, v := range proc.centralAuth.pki.nodesAcked.keysAndHash.Keys {
					nk := NodesAndKeys{
						Node: k,
						Key:  v,
					}

					sortedNodesAndKeys = append(sortedNodesAndKeys, nk)
				}

				// sort the slice based on the node name.
				// Sort all the commands.
				sort.SliceStable(sortedNodesAndKeys, func(i, j int) bool {
					return sortedNodesAndKeys[i].Node < sortedNodesAndKeys[j].Node
				})

				// Then create a hash based on the sorted slice.

				b, err := cbor.Marshal(sortedNodesAndKeys)
				if err != nil {
					er := fmt.Errorf("error: methodREQKeysAllow, failed to marshal slice, and will not update hash for public keys:  %v", err)
					proc.errorKernel.errSend(proc, message, er)
					log.Printf(" * DEBUG: %v\n", er)

					return
				}

				// Store the key in the key value map.
				hash := sha256.Sum256(b)
				proc.centralAuth.pki.nodesAcked.keysAndHash.Hash = hash

				// Store the key to the db for persistence.
				proc.centralAuth.pki.dbUpdateHash(hash[:])
				if err != nil {
					er := fmt.Errorf("error: methodREQKeysAllow, failed to store the hash into the db:  %v", err)
					proc.errorKernel.errSend(proc, message, er)
					log.Printf(" * DEBUG: %v\n", er)

					return
				}

			}()

		}
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
