# steward

## What is it ?

Command And Control anything asynchronously.

Send shell commands to control your servers by passing a message that will have guaranteed delivery if/when the subsribing node is available. Or for example send logs or metrics from an end node back to a central log subscriber.

The idea is to build and use a pure message passing architecture for the commands back and forth from nodes, where delivery is guaranteed, and where all of the processes in the system are running concurrently so if something breaks or some process is slow it will not affect the handling and delivery of the other messages in the system.

By default the system guarantees that the order of the messages are handled by the subscriber in the order they where sent. There have also been implemented a special type `NOSEQ` which will allow messages to be handled within that process in a not sequential manner. This is handy for jobs that will run for a long time, and where other messages are not dependent on it's result.

A node can be a server running any host operating system, a container living in the cloud somewhere, a rapsberry pi, or something else that needs to be controlled that have an operating system installed  . The message passing backend used is <https://nats.io>

## Why ?

With existing solutions there is often either a push or a pull kind of setup.

In a push setup the commands to be executed is pushed to the receiver, but if a command fails because for example a broken network link it is up to you as an administrator to detect those failures and retry them at a later time until it is executed successfully.

In a pull setup an agent is installed at the Edge unit, and the configuration or commands to execute locally are pulled from a central repository. With this kind of setup you can be pretty certain that sometime in the future the node will reach it's desired state, but you don't know when. And if you want to know the current state you will need to have some second service which gives you that information.

In it's simplest form the idea about using an event driven system as the core for management of Edge units is that the sender/publisher are fully decoupled from the receiver/subscriber. We can get an acknowledge if a message is received or not, and with this functionality we will at all times know the current state of the receiving end. We can also add information in the ACK message if the command sent to the receiver was successful or not by appending the actual output of the command.

## Disclaimer

All code in this repository are to be concidered not-production-ready. The code are the attempt to concretize the idea of a purely async management system where the controlling unit is decoupled from the receiving unit, and that that we know the state of all the receiving units at all times.

## Terminology

- Node: Something with an operating system that have network available. This can be a server, a cloud instance, a container, or other.
- Process: One message handler running in it's own thread with 1 subject for sending and 1 for reply.
- Message:
  - Command: Something to be executed on the message received. An example can be a shell command.
  - Event: Something that have happened. An example can be transfer of syslog data from a host.

## Features

### Messages in order

- By default the system guarantees that the order of the messages are handled by the subscriber in the order they where sent. So if a network link is down when the message is being sent, it will automatically be rescheduled at the specified interval with the given number of retries.

### Messages not in order

- There have been implemented a special type `NOSEQ` which will allow messages to be handled within that process in a not sequential manner. This is handy for jobs that will run for a long time, and where other messages are not dependent on it's result.

### Error messages from nodes

- Error messages will be sent back to the central error handler upon failure on a node.

### Message handling and threads

- The handling of all messages is done by spawning up a process for the handling the message in it's own thread. This allows us to individually down to the message level keep the state for each message both in regards to ACK's, error handling, send retries, and rerun of a method for a message if the first run was not successful.

- Processes for handling messages on a host can be restarted upon failure, or asked to just terminate and send a message back to the operator that something have gone seriously wrong. This is right now just partially implemented to test that the concept works.

- Processes on the publishing node for handling incomming messages for new nodes will automatically be spawned when needed if it does not already exist.

- Publishing processes will potentially be able to send to all nodes. It is the subscribing nodes who will limit from where and what they will receive from.

- Messages not fully processed or not started yet will be automatically handled in chronological order if the service is restarted since the current state of all the messages being processed are stored on the local node in a key value store until they are finished.

- All messages processed by a publisher will be written to a log file as they are processed, with all the information needed to recreate the same message if needed, or it can be used for auditing.

- All handling down to the process and message level are handled concurrently. So if there are problems handling one message sent to a node on a subject it will not affect the messages being sent to other nodes, or other messages sent on other subjects to the same host.

- Default timeouts to wait for ACK messages and max attempts to retry sending a message specified upon startup. This can be overridden on the message level.

- Message types of both ACK and NACK, so we can decide if we want or don't want an Acknowledge if a message was delivered succesfully.
Example: We probably want an ACK when sending some CLICommand to be executed, but we don't care for an acknowledge (NACK) when we send an "hello I'm here" event.

### Flags and configuration file

Steward supports both the use of flags/arguments set at startup, and the use of a config file. But how it is used might be a little different than how similar use is normally done.

A default config file will be created at first startup if one does not exist, with standard defaults values set. Any value also provided via a flag will also be written to the config file. If Steward is restarted the current content of the config file will be used as the new defaults. Said with other words, if you restart Steward without any flags specified the values of the last run will be read from the config file and used.

If new values are provided via flags they will take precedence over the ones currently in the config file, and they will also replace the current value in the config file, making it the default for the next restart.

The only exception from the above are the `startSubscriberX` flags which got one extra value that can be used which is the value `RST` for Reset. This will disable the specified subscriber, and also null out the array for which Nodes the subscriber will allow traffic from.

The config file can also be edited directly, making the use of flags not needed.

If just getting back to standard default for all config options needed, then delete the current config file, restart Steward, and a new config file with all the options set to it's default values will be created.

### Errors reporting

- Report errors happening on some node in to central error handler.

### Prometheus metrics

- Prometheus exporters for Metrics

### Other

- More will come. In active development.

## Howto

### Build and Run

clone the repository, then cd `./steward/cmd` and do `go build -o steward`, and run the application with `./steward --help`

### Options for running

```text
    -brokerAddress string
      the address of the message broker (default "0")
  -centralErrorLogger
      set to true if this is the node that should receive the error log's from other nodes
  -defaultMessageRetries int
      default amount of retries that will be done before a message is thrown away, and out of the system
  -defaultMessageTimeout int
      default message timeout in seconds. This can be overridden on the message level (default 10)
  -node string
      some unique string to identify this Edge unit (default "0")
  -profilingPort string
      The number of the profiling port
  -promHostAndPort string
      host and port for prometheus listener, e.g. localhost:2112 (default ":2112")
```

### How to Run

The broker for messaging is Nats-server from <https://nats.io>. Download, run it, and use the `--brokerAddress` flag on Steward to point to it.

On some central server which will act as your command and control server.

`./steward --node="central"`

One the nodes out there

`./steward --node="ship1"` & `./steward --node="ship1"` and so on.

Use the `--help` flag to get all possibilities.

#### Start subscriber flags

The start subscribers flags take a string value of which nodes that it will process messages from. Since using a flag to set a value automatically sets that value also in the config file, a value of RST can be given to turn off the subscriber.

### Message fields explanation

```go
// The node to send the message to
toNode
// The Unique ID of the message
data
// method, what is this message doing, etc. CLI, syslog, etc.
method
// Normal Reply wait timeout
timeout
// Normal Resend retries
retries
// The timeout of the new message created via a request event.
requestTimeout
// The retries of the new message created via a request event.
requestRetries
// Timeout for long a process should be allowed to operate
methodTimeout
```

### How to send a Message

Right now the API for sending a message from one node to another node is by pasting a structured JSON object into a file called `inmsg.txt` living alongside the binary. This file will be watched continously, and when updated the content will be picked up, umarshaled, and if OK it will be sent a message to the node specified in the `toNode` field.

The `method` is what defines what the event will do. The preconfigured methods are:

```go
// Execute a CLI command in for example bash or cmd.
// This is a command type, so the output of the command executed
// will directly showed in the ACK message received.
CLICommand Method = "CLICommand"
// Execute a CLI command in for example bash or cmd.
// This is an event type, where a message will be sent to a
// node with the command to execute and an ACK will be replied
// if it was delivered succesfully. The output of the command
// ran will be delivered back to the node where it was initiated
// as a new message.
CLICommandRequest Method = "CLICommandRequest"
// Execute a CLI command in for example bash or cmd.
// This is an event type, where a message will be sent to a
// node with the command to execute and an ACK will be replied
// if it was delivered succesfully. The output of the command
// ran will be delivered back to the node where it was initiated
// as a new message.
// The NOSEQ method will process messages as they are recived,
// and the reply back will be sent as soon as the process is
// done. No order are preserved.
CLICommandRequestNOSEQ Method = "CLICommandRequestNOSEQ"
// Will generate a reply for a CLICommandRequest.
// This type is normally not used by the user when creating
// a message. It is used in creating the reply message with
// request messages. It is also used when defining a process to
// start up for receiving the CLICommand request messages.
CLICommandReply Method = "CLICommandReply"
// Send text logging to some host.
// A file with the full subject+hostName will be created on
// the receiving end.
TextLogging Method = "TextLogging"
// Send Hello I'm here message.
SayHello Method = "SayHello"
// Error log methods to centralError node.
ErrorLog Method = "ErrorLog"
// Echo request will ask the subscriber for a
// reply generated as a new message, and sent back to where
// the initial request was made.
ECHORequest Method = "ECHORequest"
// Will generate a reply for a ECHORequest
ECHOReply Method = "ECHOReply"
```

NB: Both the keys and the values used are case sensitive.

#### Sending a command from one Node to Another Node

Example JSON for appending a message of type command into the `inmsg.txt` file

```json
[
    {
        
        "toNode": "ship1",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"CommandACK",
        "method":"CLICommand"
            
    }
]
```

To send specify more messages at once do

```json
[
    {
        
        "toNode": "ship1",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"CommandACK",
        "method":"CLICommand"
            
    },
    {
        
        "toNode": "ship2",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"CommandACK",
        "method":"CLICommand"
            
    }
]
```

To send a message with custom timeout and amount of retries

```json
[
    {
        
        "toNode": "ship1",
        "data": ["bash","-c","netstat -an|grep -i listen"],
        "commandOrEvent":"CommandACK",
        "method":"CLICommand",
        "timeout":3,
        "retries":3
    }
]
```

You can save the content to myfile.JSON and append it to `inmsg.txt`

`cat myfile.json >> inmsg.txt`

The content of `inmsg.txt` will be erased as messages a processed.

#### Sending a message of type Event

```json
[
    {
        "toNode": "central",
        "data": ["some message sent from a ship for testing\n"],
        "commandOrEvent":"EventACK",
        "method":"TextLogging"
    }
]
```

You can save the content to myfile.JSON and append it to `inmsg.txt`

`cat myfile.json >> inmsg.txt`

The content of `inmsg.txt` will be erased as messages a processed.

## Concepts/Ideas

### Naming

#### Subject

`<nodename>.<command/event>.<method>`

Nodename: Are the hostname of the device. This do not have to be resolvable via DNS, it is just a unique name for the host to receive the message.

Command/Event: Are type of message sent. `CommandACK`/`EventACK`/`CommandNACK`/`EventNACK`. Description of the differences are mentioned earlier.\
Info: The command/event which is called a MessageType are present in both the Subject structure and the Message structure. The reason for this is that it is used both in the naming of a subject, and in the message for knowing what kind of message it is and how to handle it.

Method: Are the functionality the message provide. Example could be `CLICommand` or `Syslogforwarding`

##### Complete subject example

For syslog of type event to a host named "ship1"

`ship1.EventACK.Syslogforwarding`

and for a shell command of type command to a host named "ship2"

`ship2.CommandACK.CLICommand`

## TODO

- FIX so it can handle multiple slices of input for inmsg.txt

- Make a scraper that first send an EventACK, and the content of the scraping is returned by a node as a new EventACK back the where the initial event originated.

- Implement a log scraper method in `tail -f` style ?

- Implement a web scraper method ?

- Encryption between Node instances and brokers.

- Authentication between node instances and brokers.

- Implement context to be able to stop processes, and message handlers.

## Overview

All parts of the system like processes, method handlers, messages, error handling are running concurrently.

If one process hangs on a long running message method it will not affect the rest of the system.

### Publisher

- A message in valid format is appended to the in pipe.
- The message is picked up by the system and put on a FIFO ringbuffer.
- The method type of the message is checked, a subject is created based on the content of the message,  and a process to handle that message type for that specific receiving node is started if it does not exist.
- The message is then serialized to binary format, and sent to the subscriber on the receiving node.
- If the message is expected to be ACK'ed by the subcriber then the publisher will wait for an ACK if the message was delivered. If an ACK was not received within the defined timeout the message will be resent. The amount of retries are defined within the message.

### Subscriber

- The subscriber will need to have a listener started on the wanted subject to be able to receive messages.
- When a message have been deserialized, it will lookup the correct handler for the method type specified within the message, and execute that handler.
- If the output of the method called is supposed to be returned to the publiser it will do so, or else it will be finish, and pick up the next message in the queue.

![overview](steward.svg)
