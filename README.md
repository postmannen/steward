# steward

Async management of Edge Cloud units.

The idea is to build and use a pure message passing architecture for the control traffic back and forth from the Edge cloud units. The message passing backend used is <https://nats.io>

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

In a push setup the commands to execute is pushed to the receiver, but if a command fails because for example a broken network link it is up to you as an administrator to detect those failures and retry them at a later time until it is executed successfully.

In a pull setup an agent is installed at the Edge unit, and the configuration or commands to execute locally are pulled from a central repository. With this kind of setup you can be pretty certain that sometime in the future the Edge unit will reach it's desired state, but you don't know when. And if you want to know the current state you will need to have some second service which gives you that.

In it's simplest form the idea about using an event driven system as the core for management of Edge units is that the sender/publisher are fully decoupled from the receiver/subscriber. We can get an acknowledge if a message is received or not, and with this functionality we will at all times know the current state of the receiving end. We can also add information in the ACK message if the command sent to the receiver was successful or not by appending the actual output of the command.

## Disclaimer

All code in this repository are to be concidered not-production-ready. The code are the attempt to concretize the idea of a purely async management system where the controlling unit is decoupled from the receiving unit, and that that we know the state of all the receiving units at all times.

## Concepts/Ideas

### Terminology

- Node: An installation of an operating system with an ip address
- Process: One message handler running in it's own thread with 1 subject for sending and 1 for reply.
- Message:
  - Command: Something to be executed on the message received. An example can be a shell command.
  - Event: Something that have happened. An example can be transfer of syslog data from a host.

### Naming

#### Subject

Subject naming are case sensitive, and can not contain the space are the tab character.

`<nodename>.<command/event>.<method>.<domain>`

Nodename: Are the hostname of the device. This do not have to be resolvable via DNS, it is just a unique name for the host to receive the message.

Command/Event: Are type of message sent. `command` or `event`. Description of the differences are mentioned earlier.\
Info: The command/event which is called a MessageType are present in both the Subject structure and the Message structure. The reason for this is that it is used both in the naming of a subject, and in the message for knowing what kind of message it is and how to handle it.

Method: Are the functionality the message provide. Example could be `shellcommand` or `syslogforwarding`

Domain: Are used to describe what domain the method are for. For example there can be several logging services on an installation, but rarely there are several logging services in place for the same Domain using the same logging method. By having a specific Domain field we will be able to differentiate several loggers having for example `method=syslogforwarding` where one might be for `domain=nginx` and another for `domain=apache`.

##### Complete subject example

For syslog of type event to a host named "ship1"

`ship1.event.syslogforwarding.nginx`

and for a shell command of type command to a host named "ship2"

`ship2.command.shellcommand.operatingsystem`

## TODO

- Timeouts. Does it makes sense to have a default timeout for all messages, and where that timeout can be overridden per message upon creation of the message.

- Check that there is a node for the specific message new incomming message, and the supervisor should create the process with the wanted subject on both the publishing and the receiving node. If there is no such node an error should be generated and processed by the error-kernel.

- Since a process will be locked while waiting to send the error on the errorCh maybe it makes sense to have a channel inside the processes error handling with a select so we can send back to the process if it should continue or not based not based on how severe the error where. This should be right after sending the error sending in the process.

- Look into adding a channel to the error messages sent from a worker process, so the error kernel can send f.ex. a shutdown instruction back to the worker.
