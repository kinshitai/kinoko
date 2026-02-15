---
name: code-blocks-example
version: 1
author: code-expert
confidence: 0.88
created: 2026-02-14
tags: [yaml, docker, config]
---

# Code Blocks with YAML-like Content

## When to Use
When your skill needs to include code blocks that contain YAML, Docker Compose, 
or other structured data that might confuse the SKILL.md parser.

## Solution
Properly escape and format code blocks to avoid parser confusion:

### YAML Configuration Files
```yaml
# This YAML content is safely inside a code block
server:
  host: localhost
  port: 8080
  settings:
    - name: "timeout"
      value: 30
    - name: "retries" 
      value: 3
  database:
    driver: postgres
    connection:
      host: db.example.com
      port: 5432
      ssl: true
```

### Docker Compose Files
```docker-compose
version: '3.8'
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    environment:
      - NGINX_HOST=example.com
      - NGINX_PORT=80
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
  
  app:
    build: .
    depends_on:
      - database
    environment:
      DATABASE_URL: "postgres://user:pass@database:5432/myapp"
    volumes:
      - .:/app
      - /app/node_modules
```

### Kubernetes YAML
```kubernetes
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp-deployment
  labels:
    app: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
      - name: myapp
        image: myapp:1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: url
```

### Configuration with Colons and Dashes
```conf
# This config format might confuse YAML parsers
listen-address: 0.0.0.0:8080
database-url: postgres://localhost:5432/mydb
redis-url: redis://localhost:6379/0
log-level: info
features:
  - authentication: enabled
  - caching: enabled  
  - metrics: disabled
```

## Gotchas
- **Parser confusion:** YAML content in code blocks might confuse the front matter parser
- **Indentation matters:** Maintain proper indentation in code blocks
- **Special characters:** Colons, dashes, and brackets in code blocks are safe when properly fenced
- **Triple backticks:** Always close your code blocks properly
- **Language hints:** Use language identifiers (`yaml`, `docker-compose`, etc.) for better syntax highlighting

## See Also
- [[yaml-best-practices]]
- [[docker-compose-patterns]]
- [[kubernetes-deployment-strategies]]