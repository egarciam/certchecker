#!/bin/bash

# Variables
 SECRET_NAME="tls-secret-1"
 NAMESPACE="default"
 CERT_FILE="tls.crt"
 KEY_FILE="tls.key"
 EXPIRY_DAYS=15
 DOMAIN="example.com"
#
# # Generate a private key
 openssl genrsa -out ${KEY_FILE} 2048
#
# # Generate a self-signed certificate valid for 15 days
 openssl req -x509 -nodes -days ${EXPIRY_DAYS} -key ${KEY_FILE} -out ${CERT_FILE} -subj "/CN=${DOMAIN}"
#
# # Encode files in base64
 BASE64_CERT=$(base64 ${CERT_FILE} | tr -d '\n')
 BASE64_KEY=$(base64 ${KEY_FILE} | tr -d '\n')
#
# # Create the Secret manifest
cat <<EOF > tls-secret-1.yaml
apiVersion: v1
kind: Secret
metadata:
  name: ${SECRET_NAME}
  namespace: ${NAMESPACE}
type: kubernetes.io/tls
data:
  tls.crt: ${BASE64_CERT}
  tls.key: ${BASE64_KEY}
EOF

echo "Secret manifest 'tls-secret.yaml' created successfully."
echo "You can apply it using: kubectl apply -f tls-secret.yaml"

#         # Clean up generated certificate and key files if you don't need them
#         # rm -f ${CERT_FILE} ${KEY_FILE}
#
