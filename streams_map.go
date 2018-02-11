package quic

import (
	"fmt"

	"github.com/costinm/quickgo/internal/flowcontrol"
	"github.com/costinm/quickgo/internal/handshake"
	"github.com/costinm/quickgo/internal/protocol"
	"github.com/costinm/quickgo/internal/wire"
)

type streamType int

const (
	streamTypeOutgoingBidi streamType = iota
	streamTypeIncomingBidi
	streamTypeOutgoingUni
	streamTypeIncomingUni
)

type streamsMap struct {
	perspective protocol.Perspective

	sender            streamSender
	newFlowController func(protocol.StreamID) flowcontrol.StreamFlowController

	outgoingBidiStreams *outgoingBidiStreamsMap
	outgoingUniStreams  *outgoingUniStreamsMap
	incomingBidiStreams *incomingBidiStreamsMap
	incomingUniStreams  *incomingUniStreamsMap
}

var _ streamManager = &streamsMap{}

func newStreamsMap(
	sender streamSender,
	newFlowController func(protocol.StreamID) flowcontrol.StreamFlowController,
	perspective protocol.Perspective,
	version protocol.VersionNumber,
) streamManager {
	m := &streamsMap{
		perspective:       perspective,
		newFlowController: newFlowController,
		sender:            sender,
	}
	var firstOutgoingBidiStream, firstOutgoingUniStream, firstIncomingBidiStream, firstIncomingUniStream protocol.StreamID
	if perspective == protocol.PerspectiveServer {
		firstOutgoingBidiStream = 1
		firstIncomingBidiStream = 4 // the crypto stream is handled separatedly
		firstOutgoingUniStream = 3
		firstIncomingUniStream = 2
	} else {
		firstOutgoingBidiStream = 4 // the crypto stream is handled separately
		firstIncomingBidiStream = 1
		firstOutgoingUniStream = 2
		firstIncomingUniStream = 3
	}
	newBidiStream := func(id protocol.StreamID) streamI {
		return newStream(id, m.sender, m.newFlowController(id), version)
	}
	newUniSendStream := func(id protocol.StreamID) sendStreamI {
		return newSendStream(id, m.sender, m.newFlowController(id), version)
	}
	newUniReceiveStream := func(id protocol.StreamID) receiveStreamI {
		return newReceiveStream(id, m.sender, m.newFlowController(id), version)
	}
	m.outgoingBidiStreams = newOutgoingBidiStreamsMap(
		firstOutgoingBidiStream,
		newBidiStream,
		sender.queueControlFrame,
	)
	// TODO(#523): make these values configurable
	m.incomingBidiStreams = newIncomingBidiStreamsMap(
		firstIncomingBidiStream,
		protocol.MaxBidiStreamID(protocol.MaxIncomingStreams, perspective),
		protocol.MaxIncomingStreams,
		sender.queueControlFrame,
		newBidiStream,
	)
	m.outgoingUniStreams = newOutgoingUniStreamsMap(
		firstOutgoingUniStream,
		newUniSendStream,
		sender.queueControlFrame,
	)
	// TODO(#523): make these values configurable
	m.incomingUniStreams = newIncomingUniStreamsMap(
		firstIncomingUniStream,
		protocol.MaxUniStreamID(protocol.MaxIncomingStreams, perspective),
		protocol.MaxIncomingStreams,
		sender.queueControlFrame,
		newUniReceiveStream,
	)
	return m
}

func (m *streamsMap) getStreamType(id protocol.StreamID) streamType {
	if m.perspective == protocol.PerspectiveServer {
		switch id % 4 {
		case 0:
			return streamTypeIncomingBidi
		case 1:
			return streamTypeOutgoingBidi
		case 2:
			return streamTypeIncomingUni
		case 3:
			return streamTypeOutgoingUni
		}
	} else {
		switch id % 4 {
		case 0:
			return streamTypeOutgoingBidi
		case 1:
			return streamTypeIncomingBidi
		case 2:
			return streamTypeOutgoingUni
		case 3:
			return streamTypeIncomingUni
		}
	}
	panic("")
}

func (m *streamsMap) OpenStream() (Stream, error) {
	return m.outgoingBidiStreams.OpenStream()
}

func (m *streamsMap) OpenStreamSync() (Stream, error) {
	return m.outgoingBidiStreams.OpenStreamSync()
}

func (m *streamsMap) OpenUniStream() (SendStream, error) {
	return m.outgoingUniStreams.OpenStream()
}

func (m *streamsMap) OpenUniStreamSync() (SendStream, error) {
	return m.outgoingUniStreams.OpenStreamSync()
}

func (m *streamsMap) AcceptStream() (Stream, error) {
	return m.incomingBidiStreams.AcceptStream()
}

func (m *streamsMap) AcceptUniStream() (ReceiveStream, error) {
	return m.incomingUniStreams.AcceptStream()
}

func (m *streamsMap) DeleteStream(id protocol.StreamID) error {
	switch m.getStreamType(id) {
	case streamTypeIncomingBidi:
		return m.incomingBidiStreams.DeleteStream(id)
	case streamTypeOutgoingBidi:
		return m.outgoingBidiStreams.DeleteStream(id)
	case streamTypeIncomingUni:
		return m.incomingUniStreams.DeleteStream(id)
	case streamTypeOutgoingUni:
		return m.outgoingUniStreams.DeleteStream(id)
	default:
		panic("invalid stream type")
	}
}

func (m *streamsMap) GetOrOpenReceiveStream(id protocol.StreamID) (receiveStreamI, error) {
	switch m.getStreamType(id) {
	case streamTypeOutgoingBidi:
		return m.outgoingBidiStreams.GetStream(id)
	case streamTypeIncomingBidi:
		return m.incomingBidiStreams.GetOrOpenStream(id)
	case streamTypeIncomingUni:
		return m.incomingUniStreams.GetOrOpenStream(id)
	case streamTypeOutgoingUni:
		// an outgoing unidirectional stream is a send stream, not a receive stream
		return nil, fmt.Errorf("peer attempted to open receive stream %d", id)
	default:
		panic("invalid stream type")
	}
}

func (m *streamsMap) GetOrOpenSendStream(id protocol.StreamID) (sendStreamI, error) {
	switch m.getStreamType(id) {
	case streamTypeOutgoingBidi:
		return m.outgoingBidiStreams.GetStream(id)
	case streamTypeIncomingBidi:
		return m.incomingBidiStreams.GetOrOpenStream(id)
	case streamTypeOutgoingUni:
		return m.outgoingUniStreams.GetStream(id)
	case streamTypeIncomingUni:
		// an incoming unidirectional stream is a receive stream, not a send stream
		return nil, fmt.Errorf("peer attempted to open send stream %d", id)
	default:
		panic("invalid stream type")
	}
}

func (m *streamsMap) HandleMaxStreamIDFrame(f *wire.MaxStreamIDFrame) error {
	id := f.StreamID
	switch m.getStreamType(id) {
	case streamTypeOutgoingBidi:
		m.outgoingBidiStreams.SetMaxStream(id)
		return nil
	case streamTypeOutgoingUni:
		m.outgoingUniStreams.SetMaxStream(id)
		return nil
	default:
		return fmt.Errorf("received MAX_STREAM_DATA frame for incoming stream %d", id)
	}
}

func (m *streamsMap) UpdateLimits(p *handshake.TransportParameters) {
	m.outgoingBidiStreams.SetMaxStream(p.MaxBidiStreamID)
	m.outgoingUniStreams.SetMaxStream(p.MaxUniStreamID)
}

func (m *streamsMap) CloseWithError(err error) {
	m.outgoingBidiStreams.CloseWithError(err)
	m.outgoingUniStreams.CloseWithError(err)
	m.incomingBidiStreams.CloseWithError(err)
	m.incomingUniStreams.CloseWithError(err)
}
