# Authorization

General Authentication and Authorization are provided by NATS on the subject level, where each node are identified with an NKEY and granted access to either publish or subscribe on subjects.

This documents is the planning and concept for how we might be able to also authorize based on the content of a message, and not just on the name of the subject.

We also want to add the feature of signature checking for all messages that it comes from a trusted node.

## Signing

The Request types we want to protect is the REQCliCommand and REQCliCommandCont. We want to have an ACL list of all the message signatures that are allowed to be executed. Signatures not in the ACL list will be discarded.

By signining the  **methodArgs** field of a message with an ed25519 private key, then putting that signature as a part of the message we can at the destination when the message is received decode the message with the source nodes public key, and then verify if the signature of the received **methodArgs**, and then allow the content to be executed or discarded.

Have a flag to turn off signing on nodes. Should same flag turn off both signing and check of signatures, or should we have one for each ?

Add a flag to do signature checking on all messages for all request types to verify the sender as a double verification on top of the NATS authorization. This is not intended to be used with the ACL list, but only verifying the signature, and that the sender is are who they claim to be.

## CentralAuth

The idea here is to have central auth database which stores all the authorizations that are allowed in the system.

The system will need processes for allowing CRUD of the auth database , and also logic to distribute the database out to the specified nodes with only the relevant parts that belongs to that node.

The signatures should be created and sent as a part of the message from a node, and should not be be populated from CentralAuth to the end nodes up front.

### Authorize an CentralAuth Server to avoid having rogue Auth Servers

This can be done at the subject level on the broker since we are using NKEY's to identify the nodes.

* Example:
  * Only CentralAuth can send REQAuthUpdate, REQCertUpdate to nodes.
  * Only CentralAuth can receive REQAddAuth or REQDelAuth.

### Hello register

Create a database of all the nodes from where we have received hello messages which is stored persistently to disk. We can then use this register as the source for what nodes are in the network, whom to ask for public keys.

NB: Nodes that don't have hello messages enabled and are not present in the hello register will not be allowed to use auth.

If a node is registered in the auth db but not present in the network we should throw a log message to the errorKernel so operators would be aware of such nodes.

DECIDE: Hello messages should contain the public key ?

### Public Keys

#### Central Store

Store all the public keys in a k/v `node -> {publicKey, valid}` on CentralAuth.

The `valid` field tells if the public key are valid, or have been revoked.

#### Request to get public keys from nodes

We could get all the nodes from the hello message register on central server.

#### Request for nodes to report their public keys on startup

If the key is the same as the one stored on the auth server we should do nothing. If it is different we should report that a new key for a node is registered and needs action to be either stored or discarded.
On the CentralAuth we need a service to verify that updating the currently stored value for a nodes public key is ok.

#### Request for pushing public keys to nodes

Public Keys should only be pushed to nodes that will receive message from source node of the public key.

#### Service for key rotation

TODO

### Auth handling and storage

#### Request for operators to add authorizations for nodes

`{Command, {[]FromNode, []ToNode}}`

#### Request for operators to remove authorizations for nodes

`{Command, {[]FromNode, []ToNode}}`

#### Store that is used as the Auth DB for knowing what is needed to be distributed where

`ToNode -> Command -> []FromNode`

* When the store is updated a new push message should be sent to all the destination nodes to update their local ACL store.

On the destination node we can implement this as a ACL map `Command -> []FromNode`, and we can lookup the public key locally when a message is received from a node to verify the signature.

* Investigate if it is possible with a concept for ALL_NODES so we are able to assign the same privileges to all nodes:
  * We will need logic to create the auth map content for a new node if one is added with a public key.
  * Add logic to remove a node auth if it's been gone for a given time.

## Node

### Local ACL store

Request type to recive ACL updates.

ACL list should be implemented as a map, map `Command -> []FromNode`, and be persisted to a K/V DB.

### Local Public Key store

Request type to recive Public Key updates.

Public Keys vs nodes should be implemented as a map, map `node -> publicKey`, and be persisted to a K/V DB.

### Verification of signatures for all request types

Flag to turn on/off signature verification for all request types.

### Verification of MethodArgs Signature against ACL

TODO
