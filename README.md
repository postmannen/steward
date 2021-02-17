# steward

Async management of Edge Cloud units.

The idea is to build and use a pure message passing architecture for the control traffic back and forth from nodes, where a node can be a server, some other host system, or a container living in the cloud, or...other. The message passing backend used is <https://nats.io>

```text
                                                                             ┌─────────────────┐
                                                                             │                 │
                                                                             │                 │
                                                                             │ ┌────────────┐  │
                                                                             │ │ Edge Unit  │  │
      ┌─────────────────┐                 ┌─────────────────┐                │ └────────────┘  │
      │                 │                 │                 │ ────Event────▶ │                 │
      │  ┌────────────┐ │ ─────Event────▶ │  ┌───────────┐  │ ◀────ACK ───── │                 │
      │  │ Management │ │                 │  │  Message  │  │                └─────────────────┘
      │  │  station   │ │                 │  │  broker   │  │                ┌─────────────────┐
      │  │            │ │                 │  └───────────┘  │                │                 │
      │  └────────────┘ │ ◀─────ACK ───── │                 │ ────Event────▶ │                 │
      │                 │                 │                 │ ◀────ACK ───── │  ┌────────────┐ │
      └─────────────────┘                 └─────────────────┘                │  │ Edge Unit  │ │
                                                                             │  └────────────┘ │
                                                                             │                 │
                                                                             │                 │
                                                                             └─────────────────┘
```

Why ?

With existing solutions there is often either a push or a pull kind of setup.

In a push setup the commands to be executed is pushed to the receiver, but if a command fails because for example a broken network link it is up to you as an administrator to detect those failures and retry them at a later time until it is executed successfully.

In a pull setup an agent is installed at the Edge unit, and the configuration or commands to execute locally are pulled from a central repository. With this kind of setup you can be pretty certain that sometime in the future the Edge unit will reach it's desired state, but you don't know when. And if you want to know the current state you will need to have some second service which gives you that.

In it's simplest form the idea about using an event driven system as the core for management of Edge units is that the sender/publisher are fully decoupled from the receiver/subscriber. We can get an acknowledge if a message is received or not, and with this functionality we will at all times know the current state of the receiving end. We can also add information in the ACK message if the command sent to the receiver was successful or not by appending the actual output of the command.

## Disclaimer

All code in this repository are to be concidered not-production-ready. The code are the attempt to concretize the idea of a purely async management system where the controlling unit is decoupled from the receiving unit, and that that we know the state of all the receiving units at all times.

## Features

- Send a message with some command or event to some node and be sure that the message is delivered. If for example the network link is down, the message will be retried until succesful.

- Error messages can or will be sent back to the central upon failure.

- The handling of all messages is done by spawning up a process for the handling in it's own thread, this allows us for individually keep the state for each message both in regards to ACK an Errors handled all within the thread that was spawned for owning the message (if that made sense ??).

- Processes for handling messages on a host can and might be automatically restarted upon failure, or asked to just terminate and send a message back to the operator that something have gone seriously wrong. This is right now just partially implemented to test that the concept works.

- Processes on the publishing node for handling incomming messages for new nodes will automatically be spawned when needed if it does not already exist.

- Publishers will potentially be able to send to all nodes. It is the subscribing nodes who will limit where and what they will receive from.

- Messages not fully processed or not started yet will be automatically handled in chronological order if the service is restarted.

- All messages processed by a publisher will be written to a log file as they are processed, with all the information needed to recreate the same message.

- All handling down to the process and message level are handled concurrently. So if there are problems handling one message sent to a node on a subject it will not affect the messages being sent to other nodes, or other messages sent on other subjects to the same host.

- More will come. In active development.

## Concepts/Ideas

### Terminology

- Node: Something with an operating system that have network available. This can be a server, a cloud instance, a container, or other.
- Process: One message handler running in it's own thread with 1 subject for sending and 1 for reply.
- Message:
  - Command: Something to be executed on the message received. An example can be a shell command.
  - Event: Something that have happened. An example can be transfer of syslog data from a host.

### Naming

#### Subject

Subject naming are case sensitive, and can not contain the space are the tab character.

`<nodename>.<command/event>.<method>`

Nodename: Are the hostname of the device. This do not have to be resolvable via DNS, it is just a unique name for the host to receive the message.

Command/Event: Are type of message sent. `command` or `event`. Description of the differences are mentioned earlier.\
Info: The command/event which is called a MessageType are present in both the Subject structure and the Message structure. The reason for this is that it is used both in the naming of a subject, and in the message for knowing what kind of message it is and how to handle it.

Method: Are the functionality the message provide. Example could be `shellcommand` or `syslogforwarding`

##### Complete subject example

For syslog of type event to a host named "ship1"

`ship1.event.syslogforwarding`

and for a shell command of type command to a host named "ship2"

`ship2.command.shellcommand`

## TODO

- Implement a message type where no ack is needed. Use case can be nodes sending "hi, I'm here" (ping) messages to central server.

- Timeouts. Does it makes sense to have a default timeout for all messages, and where that timeout can be overridden per message upon creation of the message.
Implement the concept of timeouts/TTL for messages.

- **Implemented**
Check that there is a node for the specific message new incomming message, and the supervisor should create the process with the wanted subject on the publishing.

- **Implemented**
Since a process will be locked while waiting to send the error on the errorCh maybe it makes sense to have a channel inside the processes error handling with a select so we can send back to the process if it should continue or not based not based on how severe the error where. This should be right after sending the error sending in the process.

- **Implemented**
Look into adding a channel to the error messages sent from a worker process, so the error kernel can send f.ex. a shutdown instruction back to the worker.

- Prometheus exporters for metrics.

- Go through all processes and check that the error is handled correctly, and also reported back on the error subject to the master supervisor.

- **Implemented**
Implement the code running inside of each process as it's own function type that get's passed into the spawn process method up on creation of a new method.

## Howto

### Build and Run

clone the repository, then cd `./steward/cmd` and do `go build -o steward`, and run the application with `./steward --help`

### Options for running

```bash
  -brokerAddress string
    the address of the nats message broker (default "0")
  -node string
    some unique string to identify this Edge unit (default "0")
  -profilingPort string
    The number of the profiling port
```

### How to Run

On some central server which will act as your command and control server.

`./steward --node="central"`

One the nodes out there

`./steward --node="ship1"` & `./steward --node="ship1"` and so on.

### Sending messages with commands or events

Right now there are to types of messages.

- Commands, are some command you want to run on a node, wait for it to finish, get an ACK back with the result contained in the ACK message. Think like running a shell command on some remote host.
- Events, are something you just want to deliver, and wait to get an ACK back that it was delivered succesfully. Think like forwarding logs to some host, you just want to be sure it was delivered.

### How to send a Message

Right now the API for sending a message from one node to another node is by pasting a structured JSON object into a file called `inmsg.txt` living alongside the binary. This file will be watched continously, and when updated the content will be picked up, parsed, and if OK it will be sent a message to the node specified.

Currently there is one Command message type for running shell comands and one event message type for sending logs implemented. More will come.

#### Sending a command from one Node to Another Node

Example JSON for pasting a message of type command into the `inmsg.txt` file

```json
[
    {
        
        "toNode": "ship1",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"command",
        "method":"shellCommand"
            
    }
]
```

To send specify more messages at once do

```json
[
    {
        
        "toNode": "ship1",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"command",
        "method":"shellCommand"
            
    },
    {
        
        "toNode": "ship2",
        "data": ["bash","-c","ls -l ../"],
        "commandOrEvent":"command",
        "method":"shellCommand"
            
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
        "commandOrEvent":"event",
        "method":"textLogging"
    }
]
```

You can save the content to myfile.JSON and append it to `inmsg.txt`

`cat myfile.json >> inmsg.txt`

The content of `inmsg.txt` will be erased as messages a processed.
