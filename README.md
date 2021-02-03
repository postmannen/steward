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

Command/Event: Are type of message sent. `command` or `event`. Description of the differences are mentioned earlier.

Method: Are the functionality the message provide. Example could be `shellcommand` or `syslogforwarding`

Domain: Are used to describe what domain the method are for. For example there can be several logging services on an installation, but rarely there are several logging services in place for the same Domain using the same logging method. By having a specific Domain field we will be able to differentiate several loggers having for example `method=syslogforwarding` where one might be for `domain=nginx` and another for `domain=apache`.

##### Complete subject example

For syslog of type event to a host named "ship1"

`ship1.event.syslogforwarding.nginx`

and for a shell command of type command to a host named "ship2"

`ship2.command.shellcommand.operatingsystem`
