# Nats-Server configuration

## Server config with nkey authentication

```config
port: 4222
tls {
  cert_file: "/Users/bt/tmp/autocert/ww.steward.raalabs.tech/ww.steward.raalabs.tech.crt"
  key_file: "/Users/bt/tmp/autocert/ww.steward.raalabs.tech/ww.steward.raalabs.tech.key"
}


authorization: {
    users = [
        {
            # central
            nkey: <USER_NKEY_HERE>
            permissions: {
                publish: {
			allow: ["ww.>","errorCentral.>"]
		}
            subscribe: {
			allow: ["ww.>","errorCentral.>"]
		}
            }
        }
        {
            # mixer
            nkey: <USER_NKEY_HERE>
            permissions: {
                publish: {
                        allow: ["central.>"]
                }
                subscribe: {
                        allow: ["central.>","mixer.>"]
                }
            }
        }
        {
            # node10
            nkey: <USER_NKEY_HERE>
            permissions: {
                publish: {
                        allow: ["ww.central.>","errorCentral.>","ww.morningconductor.>"]
                }
                subscribe: {
                        allow: ["ww.central.>","ww.morningconductor.>"]
                }
            }
        }
    ]
}
```

The official docs for nkeys can be found here <https://docs.nats.io/nats-server/configuration/securing_nats/auth_intro/nkey_auth>.

Generate private (seed) and public (user) key pair:

`nk -gen user -pubout`

Generate a public (user) key from a private (seed) key file called `seed.txt`.

`nk -inkey seed.txt -pubout > user.txt`

## Leafnode config

Nats-server version need to be greater than v2+ for leafnode functionality.

### Leafnode config options with certificates

<https://docs.nats.io/nats-server/configuration/leafnodes/leafnode_conf>

<https://docs.nats.io/nats-server/configuration/leafnodes>

### Central server node config

```conf
leafnodes {
    port: 7422

    tls {
        cert_file: "</some/location/mysite.com>.crt"
        key_file: "</some/location/mysite.com>.crt.key"
    }

    authorization {
        user: leaf
        password: secret
    }
}
```

### Spoke leaf node config

On the spokes we set the listen port to `4111` instead of the default `4222`.

```conf
port: 4111
leafnodes {
  remotes: [
    {
      urls: [
        tls://leaf:secret@<mysite.com>:7422
      ]
    }
  ]
}
```

## Leafnode config with self signed certificates

mkdir ca
cd ca

Generate key for rootCA

`openssl genrsa -out rootCA.key 2048`

Generate the certificate for the rootCA

`openssl req -new -x509 -nodes -key rootCA.key -sha256 -days 1024 -out rootCA.pem`

Genereate directory structure for the node certificates. NB: Normally this would not be done on the same place. Each node would only create a key and a CSR which would be sent to the rootCA, and the rootCA would create the .pem file for the node, and the .pem file would them be sent back to the node. The rootCA should never know the key's of the nodes. But since we're doing the test on a single machine, folders are used to simulate nodes.

```bash
mkdir central
mkdir ship1
mkdir ship2
```

Generate private keys for nodes

```bash
openssl genrsa -out ship1/ship1.key 2048
openssl genrsa -out ship2/ship2.key 2048
openssl genrsa -out central/central.key 2048
```

Generate signing requests

```bash
openssl req -new -key central/central.key -out central/central.csr
openssl req -new -key ship1/ship1.key -out ship1/ship1.csr
openssl req -new -key ship2/ship2.key -out ship2/ship2.csr
```

Generate certificates for nodes

```bash
openssl x509 -req -in ship1/ship1.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out ship1/ship1.pem -days 1024 -sha256
openssl x509 -req -in ship2/ship2.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out ship2/ship2.pem -days 1024 -sha256
openssl x509 -req -in central/central.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out central/central.pem -days 1024 -sha256
```

If the certificates needs to be generated for ip adresses and not FQDN, we need to add the IP SAN in an openssl.cnf file, and reference the block where it is specified when we generate the node certificate.

Copy the standard `openssl.conf` file from the system, so we can use it as a template.

`cp /usr/local/etc/openssl/openssl.cnf ./`

Open `openssl.conf` for editing, and under `[ v3_req ]` add

`subjectAltName = IP:10.0.0.124`

```bash
openssl x509 -req -in central/central.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out central/central.pem -days 1024 -sha256 -extfile ./openssl.cnf -extensions v3_req
openssl x509 -req -in ship1/ship1.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out ship1/ship1.pem -days 1024 -sha256 -extfile ./openssl.cnf -extensions v3_req
openssl x509 -req -in ship2/ship2.csr -CA rootCA.pem -CAkey rootCA.key -CAcreateserial -out ship2/ship2.pem -days 1024 -sha256 -extfile ./openssl.cnf -extensions v3_req
```

Verify with:

```bash
openssl x509 -text -noout -in central/central.pem
```

### Self signed certificates

### Central config

```conf
leafnodes {
    port: 7422

    tls {
        cert_file: "/Users/bt/tmp/ca/central/central.pem"
        key_file: "/Users/bt/tmp/ca/central/central.key"
        ca_file: "/Users/bt/tmp/ca/rootCA.pem"
    }

    authorization {
        user: leaf
        password: secret
    }
}
```

### Spoke leaf config

```conf
port: 4111
leafnodes {
  remotes: [
    {
      urls: [
        tls://leaf:secret@10.0.0.124:7422
      ]
      tls {
        ca_file: "/Users/bt/tmp/ca/rootCA.pem"
      }
    }
  ]
}
```
