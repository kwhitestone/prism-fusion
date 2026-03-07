docker kill prism-fusion
docker rm prism-fusion
docker run -d \
  --name prism-fusion \
  --env-file .vscode/.env \
  -p 18080:8080 \
  prism-fusion:latest
docker logs -f prism-fusion