#!/bin/bash
#user=$(LC_ALL=C tr -dc 'A-Za-z0-9!"#$%&'\''()*+,-./:;<=>?@[\]^_`{|}~' </dev/urandom | head -c 13 ; echo)
#pw=$(LC_ALL=C tr -dc 'A-Za-z0-9!"#$%&'\''()*+,-./:;<=>?@[\]^_`{|}~' </dev/urandom | head -c 13 ; echo)

webuser=$(
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 13
    echo
)
webpw=$(
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 13
    echo
)
addr=$(echo $(ifconfig wg0 2>/dev/null | awk '/inet addr:/ {print $2}' | sed 's/addr://'))
port=$(shuf -i 40000-40999 -n 1)
timeout=$1
realuser=$2

if id "$realuser" &>/dev/null; then
    echo "$realuser found"
else
    echo "$realuser not found, creating user.."
    useradd -m $realuser
fi

echo "url: https://$addr:$port"
echo "webuser: $webuser, webpw: $webpw"

timeout $timeout sudo -H -u $realuser /opt/gotty/gotty --tls-crt /opt/gotty/.gotty.crt --tls-key /opt/gotty/.gotty.key --once -a $addr -p $port -t -c $webuser:$webpw -w /bin/bash 2>&1
