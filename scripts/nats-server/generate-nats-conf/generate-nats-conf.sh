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
    ]
}
EOF
