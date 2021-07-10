## inject rtmp media into mediasoup


### build mediasoup 

1. git clone https://github.com/versatica/mediasoup.git

2. cd mediasoup/worker && make

### build and run go app

1. go build -mod=vendor

2. MEDIASOUP_WORKER_BIN="path to mediasoup-worker" ./r2wdemo

### build and run web app

1. cd web

2. npm install && npm start

3. open http://localhost:3001


### opus support(rtmp source)

OPUS are not supported in rtmp, however, we can still transfer opus packets on rtmp, most of rtmp servers will relay this kind of packets to rtmp clients.

Apply this [patch](https://github.com/kn007/patch/blob/master/ffmpeg-let-rtmp-flv-support-hevc-h265-opus.patch) for ffmpeg, 
then you can use ffmpeg transfer opus packets on rtmp.

   1. setup a rtmp server, I use lal for testing
   2. ffmpeg -re -i xx.mp4 -c:v copy -ar 48000 -ac 2 -c:a libopus -f flv rtmp://localhost/live/xx


Playing "rtmp://localhost/live/xx" in this demo, you will hear sound.

### dependencies

1. https://github.com/q191201771/lal.git
2. https://github.com/versatica/mediasoup.git
3. https://github.com/jiyeyuran/mediasoup-go.git