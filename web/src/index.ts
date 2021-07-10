/* eslint-disable no-console */
import { Device } from 'mediasoup-client';
//import { MediaKind, RtpCapabilities, RtpParameters } from 'mediasoup-client/lib/RtpParameters';
import { RtpCapabilities, RtpParameters } from 'mediasoup-client/lib/RtpParameters';
//import { DtlsParameters, TransportOptions, Transport } from 'mediasoup-client/lib/Transport';
import { TransportOptions, Transport } from 'mediasoup-client/lib/Transport';
import { ConsumerOptions } from 'mediasoup-client/lib/Consumer';


interface ServerMessage1 {
	flag:'s1';
	pid:string;
	routerrc:RtpCapabilities;
	recvtransoption:TransportOptions;
}

interface ServerMessage2 {
	flag:'s2';
	cid:string;
	kind:string;
	crtpp:RtpParameters;
}

type ServerMessage = 
| ServerMessage1
| ServerMessage2;


async function start_session()
{
	console.log("start session")
	const receivePreview = document.querySelector('#preview-receive') as HTMLVideoElement;

	receivePreview.onloadedmetadata = () =>
	{
		receivePreview.play();
	};

	let receiveMediaStream: MediaStream | undefined;


	const ws = new WebSocket('ws://localhost:3000/ws');
	
	ws.addEventListener('open', function () {
		const rtmpurl = document.getElementById('idRtmpUrl') as HTMLInputElement;
		ws.send(rtmpurl.value);
	});


	const device = new Device();
	let consumerTransport: Transport | undefined;
	let pid:string;

	{
		ws.onmessage = async (message) => 
		{
			let decodedMessage : ServerMessage = JSON.parse(message.data);
			switch (decodedMessage.flag)
			{
				case 's1': {
					console.log(decodedMessage)
			        //servermessage1
			        await device.load({
			        	routerRtpCapabilities : decodedMessage.routerrc
			        });
					console.log("device load done")
					console.log(decodedMessage.routerrc)
			        ws.send(JSON.stringify({
			        	clientcap: device.rtpCapabilities,
			        }));
					pid = decodedMessage.pid;

			        consumerTransport = device.createRecvTransport(
			        	decodedMessage.recvtransoption
			        );

					console.log("consumertransport")
					console.log(consumerTransport)

			        consumerTransport
			        	.on('connect', ({ dtlsParameters }, success) =>
			        	{
							console.log("recv transport on connect");
							console.log(dtlsParameters);
							success(undefined)

			        		ws.send(JSON.stringify({
			        			dtlsp:dtlsParameters
			        		}));
			        	});

					consumerTransport.on('connectionstatechange', (connectionState) => {
						console.log("connecdtion state change");
						console.log(connectionState);

					});



					break;
				}
				case 's2': {
					//servermessage2
					const consumer = await (consumerTransport as Transport).consume(
						{
							id:decodedMessage.cid,
							producerId:pid,
							kind:'video',
							rtpParameters:decodedMessage.crtpp
						} as ConsumerOptions
					);


					if (receiveMediaStream)
					{
						receiveMediaStream.addTrack(consumer.track);
						receivePreview.srcObject = receiveMediaStream;
					}
					else
					{
						receiveMediaStream = new MediaStream([ consumer.track ]);
						receivePreview.srcObject = receiveMediaStream;
					}
					break;
				}

			}

		}
	}
	ws.onerror = console.error;
}

function init() {
	let btn = document.getElementById("startb");
	if (btn)
		btn.addEventListener("click", () => start_session());
}

init();
