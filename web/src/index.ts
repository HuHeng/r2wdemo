/* eslint-disable no-console */
import { Device } from 'mediasoup-client';
import { RtpCapabilities, RtpParameters } from 'mediasoup-client/lib/RtpParameters';
import { TransportOptions, Transport } from 'mediasoup-client/lib/Transport';
import { ConsumerOptions } from 'mediasoup-client/lib/Consumer';


interface ServerMessage1 {
	flag:'s1';
	vpid:string;
	apid:string;
	routerrc:RtpCapabilities;
	recvtransoption:TransportOptions;
}

interface ServerMessage2 {
	flag:'s2';
	vcid:string;
	acid:string;
	vcrtpp:RtpParameters;
	acrtpp:RtpParameters;
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
	let vpid:string;
	let apid:string;

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
					vpid = decodedMessage.vpid;
					apid = decodedMessage.apid;

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
					const vconsumer = await (consumerTransport as Transport).consume(
						{
							id:decodedMessage.vcid,
							producerId:vpid,
							kind:'video',
							rtpParameters:decodedMessage.vcrtpp
						} as ConsumerOptions
					);

					const aconsumer = await (consumerTransport as Transport).consume(
						{
							id:decodedMessage.acid,
							producerId:apid,
							kind:'audio',
							rtpParameters:decodedMessage.acrtpp
						} as ConsumerOptions
					);


					receiveMediaStream = new MediaStream([ aconsumer.track ]);
					receiveMediaStream.addTrack(vconsumer.track);
					receivePreview.srcObject = receiveMediaStream;
					//receiveMediaStream = new MediaStream([ vconsumer.track ]);
					//receiveMediaStream.addTrack(vconsumer.track);
					//receivePreview.srcObject = receiveMediaStream;

					//if (receiveMediaStream)
					//{
					//	receiveMediaStream.addTrack(consumer.track);
					//	receivePreview.srcObject = receiveMediaStream;
					//}
					//else
					//{
					//	receiveMediaStream = new MediaStream([ consumer.track ]);
					//	receivePreview.srcObject = receiveMediaStream;
					//}
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
