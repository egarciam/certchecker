# Create k8s certificates for testing 
Para probar certchecker creamos certificados adhoc para el caso

## Generate the certificate
Run the following commands in your terminal to generate the certificate and private key:
```
# Generate a private key
openssl genrsa -out tls.key 2048

# Generate a self-signed certificate valid for 15 days
openssl req -x509 -nodes -days 15 -key tls.key -out tls.crt -subj "/CN=example.com"

```
Then, encode the tls.key and tls.crt files to base64 for Kubernetes:
```
# Base64 encode the private key
base64 tls.key | tr -d '\n'

# Base64 encode the certificate
base64 tls.crt | tr -d '\n'
```
## Kubernetes Secret Manifest
Replace <BASE64_ENCODED_CERT> and <BASE64_ENCODED_KEY> with the base64-encoded content from the above commands
```
apiVersion: v1
kind: Secret
metadata:
  name: tls-secret
  namespace: default
type: kubernetes.io/tls
data:
  tls.crt: <BASE64_ENCODED_CERT>
  tls.key: <BASE64_ENCODED_KEY>
```
Usar el programa `faketime` cuando queramos crear certificados ya expirados