# Nats-Server configuration

## Leafnode config

Nats-server version need to be greater than v2+ for leafnode functionality.

### Leafnode config options

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
