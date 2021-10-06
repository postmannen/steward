#!/bin/bash

# Loop over all lines of host ip addresses specified in the hosts.txt
# file, and execute the the script given as the input argument. If no
# script is given as argument the scriptName specified below will be
# used.
# This script will run until done, to kill it use
# > CTRL+z
# > pkill -f loop-wrapper
###
# Info: change out the script in scriptName below with the executable
# script you want copied to host, and executed.
###
# Environmental variables can be passed on to the host if needed by
# embedding them directly in the ssh command. See below.
#
# sshOut=$(ssh -o ConnectTimeout=$sshTimeout -n -i $idRsaFile "$sshUser"@"$ipAddress" "sudo bash -c 'export NODENAME="$name"; $scpDestinationFolder/$scriptName'" 2>&1)
# Here we are exporting NODENAME set to the current value of $name when
# starting the ssh session, and then the script called on host can take
# use of it directly.

if [ -z "$1" ]; then
    echo "No script to call supplied"
    echo "Usage: ./loop-wrapper <my-script> <id.rsa>"
    exit 1
else
    scriptSourcePath=$(dirname "$1")"/"
    echo "scriptSourcePath=$scriptSourcePath"
    scriptName=$(basename "$1")
    echo "scriptName=$scriptName"
fi

if [ -z "$2" ]; then
    echo "No id.rsa file supplied"
    echo "Usage: ./loop-wrapper <my-script> <id.rsa>"
    exit 1
else
    idRsaFile=$2
fi

pingTimeout=3

statusLog="./status.log"
touch $statusLog

notDoneFile="./notdone.tmp"
doneFile="./done.log"
touch $doneFile

# The file containing the ip adresses to work with.
# One address per line.
hostsFile="./hosts.txt"

sshTimeout=30
sshUser=ansible
scpDestinationFolder=/home/$sshUser

# Loop until notDoneFile is empty. Checked in the end of loop.
while true; do
    touch $notDoneFile

    # Read from the hosts.txt file one line at a time
    # and execute..
    while IFS=, read -r ipAddress name; do
        if [[ -z "$name" ]]; then
            echo "error: missing name for ip $ipAddress in hosts.txt. Should be <ip address>,<node name>"
            exit 1
        fi

        echo working on :"$ipAddress","$name"

        # Check if host is available
        if ping "$ipAddress" -c 1 -t $pingTimeout; then
            echo ok info:ping ok:"$name", "$ipAddress" >>$statusLog
            ## Put whatever to execute here...
            ## copy files&scripts to run on host.
            ## Execute the copied files/scripts on host.
            # ----------------------------------------------
            # scp script file.
            if scp -o ConnectTimeout=$sshTimeout -o StrictHostKeyChecking=no -i $idRsaFile "$scriptSourcePath$scriptName" "$sshUser"@"$ipAddress":; then
                # Execute script file copied via ssh.
                sshOut=$(ssh -o ConnectTimeout=$sshTimeout -n -i $idRsaFile "$sshUser"@"$ipAddress" "sudo bash -c 'export NODENAME=$name; $scpDestinationFolder/$scriptName'" 2>&1)

                if [ $? -eq 0 ]; then
                    echo ok info:succesfully executing ssh command: "$name", "$ipAddress"
                    echo ok info:succesfully executing ssh command: "$name", "$ipAddress" >>$statusLog
                else
                    echo error:failed executing ssh command: "$sshOut", "$ipAddress", "$name"
                    echo error:failed executing ssh command: "$sshOut","$ipAddress", "$name" >>$statusLog
                    echo "$ipAddress","$name" >>$notDoneFile
                    continue
                fi
            else
                echo error:failed scp copying, not running ssh command:"$ipAddress", "$name"
                echo error:failed scp copying, not running ssh command:"$ipAddress", "$name" >>$statusLog
                echo "$ipAddress","$name" >>$notDoneFile
                continue

            fi
            # ----------------------------------------------

            echo "info: all done for host: " "$ipAddress", "$name"
            echo "info: all done for host: " "$ipAddress", "$name" >>$statusLog

            echo "$ipAddress","$name","$sshOut" >>$doneFile
        else
            echo error: no ping reply: "$ipAddress", "$name"
            echo error: no ping reply: "$ipAddress", "$name" >>$statusLog

            echo "$ipAddress","$name" >>$notDoneFile
            continue
        fi

    done <./hosts.txt

    # Check if notdone file is empty, loop if not empty,
    # and break out and exit if empty.
    if [ -s "$notDoneFile" ]; then
        # The file is not-empty.
        echo "the notDone file is not-empty, so we're looping"
        cp $notDoneFile "$hostsFile"
        rm $notDoneFile
    else
        # The file is empty, meaning.. we're done.
        echo "info: all done for all hosts, exiting: "
        echo "info: all done for all hosts, exiting: " >>$statusLog

        cp $notDoneFile "$hostsFile"
        rm $notDoneFile
        break
    fi

    sleep 2

done
