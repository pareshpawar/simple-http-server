# Simple HTTP Server in Go as a Container Image

`simple-http-server` returns IP address, headers and many other request data. Use this as a test app to to deploy in your servers or practice to deploy.

### How it works?

- Starts listening on port 8081 for HTTP traffic
- On every request it returns following things:
  - Request Type
  - Hostname or Host of request
  - Local IP of Container
  - Remote IP
  - All Request Headers
  - And Environment Variable called `YOUR_ENV`

### How to use it?

- I've hosted this image on [Docker Hub](https://hub.docker.com/r/pareshpawar/simple-http-server) or you can build it yourself if you want to.

  ```bash
  sudo docker run -p 8081:8081 pareshpawar/simple-http-server
  ```

- Optionally, forward port via your proxy or load balancer.

#### To Do

- [x] Make std output/logs colored and pretty 😅
- [ ] Add Environment Variable to switch text output to html output
- [ ] Serve a html pages from a external directory
- [ ] Add volume Env var to serve volume as http dir
- [ ] create github actions for docker image build
