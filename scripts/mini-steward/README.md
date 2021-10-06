# mini-steward

Loop over all lines of host ip addresses specified in the hosts.txt
file, and execute the the script with the id.rsa given as the input argument.
This script will run until done, to kill it use:

```bash
CTRL+z
pkill -f loop-wrapper
```

## Overview

- This is a helper script that will copy a script you want to run on one or more nodes, and execute it.
- The result of running the script will be returned in the `status.log` file.
- The script wil continue running until execution on all nodes are succesful.
- The result when successful run on a host will be written to the `done.log` file, with the output specified.

## Usage

Specify the the hosts to execute the script on in the `hosts.txt` file, notation `ip-address,node-name` separated by new lines.

Example :

```text
10.0.0.1,node1
10.0.0.2,node2
```

And start running the script with:

```bash
./loop-wrapper <script-to-copy-and-execute> <path-to-id.rsa file>
```

### The script to be executed

To get the output written to `done.log` from the script that have been copied and ran on the nodes, it should write the desired output to `STDOUT` as it's last thing before calling `exit 0`.

The same goes for error messages.

See more details in the `test.sh` file for an example.
