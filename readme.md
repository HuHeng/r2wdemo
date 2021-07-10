## inject media into mediasoup


### build mediasoup 

1. [mediasoup](https://github.com/versatica/mediasoup.git)

2. cd mediasoup/worker && make

### build and run go app

1. go build -mod=vendor

2. MEDIASOUP_WORKER_BIN="path to mediasoup-worker" ./injectmediasoup

### build and run web app

1. cd web

2. npm install && npm start

3. open http://localhost:3001
