#!/bin/sh

set -e

SSL_DIR="${SSL_DIR:-/etc/topograph/ssl}"

CA_KEY="${SSL_DIR}/ca-key.pem"
CA_CERT="${SSL_DIR}/ca-cert.pem"

SRV_KEY="${SSL_DIR}/server-key.pem"
SRV_CSR="${SSL_DIR}/server.csr"
SRV_CERT="${SSL_DIR}/server-cert.pem"

mkdir -p "${SSL_DIR}"
openssl genrsa -out ${CA_KEY} 4096
openssl req -x509 -new -nodes -key ${CA_KEY} -sha512 -out ${CA_CERT} -subj "/CN=*" -days 365
openssl genrsa -out ${SRV_KEY} 4096
openssl req -new -key ${SRV_KEY} -out ${SRV_CSR} -subj "/O=nvidia/CN=localhost" -days 365
openssl x509 -req -in ${SRV_CSR} -CA ${CA_CERT} -CAkey ${CA_KEY} -CAcreateserial -out ${SRV_CERT} -sha512
openssl verify -CAfile ${CA_CERT} ${SRV_CERT}
