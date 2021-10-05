#!/bin/bash

cat >./nats.conf <<EOF
port: 4022
tls {
  cert_file: "/app/le.crt"
  key_file: "/app/le.key"
}

authorization: {
    users = [
        {
    
        }
        {
            # central
            nkey: FILL-IN-CENTRAL-USER-KEY-HERE
            permissions: {
                publish: {
                        allow: ["FILL-IN-ORGANISATION-HERE.>","errorCentral.>","central.>"]
                }
            subscribe: {
                        allow: ["FILL-IN-ORGANISATION-HERE.>","errorCentral.>","central.>"]
                }
            }
        }
    ]
}
EOF
