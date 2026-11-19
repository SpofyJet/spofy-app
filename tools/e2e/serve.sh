set -u
mkdir -p /tmp/tls
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 -keyout /tmp/tls/key.pem -out /tmp/tls/cert.pem -subj /CN=sub.lab.test -addext subjectAltName=DNS:sub.lab.test >/dev/null 2>&1
echo "127.0.0.1 sub.lab.test" >> /etc/hosts
export SSL_CERT_FILE=/tmp/tls/cert.pem
export SPOFY_STATE_KEY=$(head -c 32 /dev/urandom | base64)
python3 /e2e/serve.py
