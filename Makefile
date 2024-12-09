# Create database
db:
	@docker run -d --name graphify-db -p 9630:8529 -e ARANGO_ROOT_PASSWORD="0Jt8Vsyp" arangodb:3.11.8

# Generate TLS certificate and key
tls:
	@openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"

# Run the example
example: tls
	@go run cmd/example/main.go --db http://localhost:9630 --user root --pass 0Jt8Vsyp
