# Relay configuration

Example config where we allow **ship1** to send a message to **ship2** relayed via **central**.

```json
port: 4222

authorization: {
    users = [
        {
            # central
            nkey: <central key here>
            permissions: {
                publish: {
                    allow: [">","errorCentral.>"]
                }
                subscribe: {
                    allow: [">","errorCentral.>"]
                }
            }
        }
        {
            # ship1
            nkey: <ship1 key here>
            permissions: {
                publish: {
                        allow: ["central.>","errorCentral.>","ship1.>","ship2.central.REQRelay.EventACK"]
                        # deny: ["*.REQRelay.>"]
                }
                subscribe: {
                        allow: ["central.>","ship1.>","errorCentral.REQErrorLog.EventACK.reply","*.central.REQRelay.EventACK.reply"]
                }
            }
        }
        {
            # ship2
            nkey: <ship2 key here>
            permissions: {
                publish: {
                        allow: ["central.>","errorCentral.>","ship2.>"]
                }
                subscribe: {
                        allow: ["central.>","ship2.>","errorCentral.REQErrorLog.EventACK.reply"]
                }
            }
        }
    ]
}
```
