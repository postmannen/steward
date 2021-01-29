# steward

Async management of Edge units.

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

`<nodename>.<command/event>.<method>`

Nodename: Are the hostname of the device. This do not have to be resolvable via DNS, it is just a unique name for the host to receive the message.

Command/Event: Are type of message sent. `command` or `event`. Description of the differences are mentioned earlier.

Method: Are the functionality the message provide. Example could be `shellcommand` or `syslogforwarding`

##### Complete subject example

For syslog of type event to a host named "ship1"

`ship1.event.syslogforwarding`

and for a shell command of type command to a host named "ship2"

`ship2.command.shellcommand`
