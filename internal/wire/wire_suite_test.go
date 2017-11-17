package wire

import (
	"bytes"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestCrypto(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Wire Suite")
}

const (
	// a QUIC version that uses big endian encoding
	versionBigEndian = protocol.Version39
	// a QUIC version that uses the MAX_DATA / MAX_STREAM_DATA and BLOCKED / STREAM_BLOCKED frames
	versionMaxDataFrame = protocol.VersionTLS
	// a QUIC version that uses the IETF frame types
	versionIETFFrames = protocol.VersionTLS
)

func encodeVarInt(i uint64) []byte {
	b := &bytes.Buffer{}
	utils.WriteVarInt(b, i)
	return b.Bytes()
}

var _ = BeforeSuite(func() {
	Expect(utils.GetByteOrder(versionBigEndian)).To(Equal(utils.BigEndian))
	Expect(utils.GetByteOrder(versionMaxDataFrame)).To(Equal(utils.BigEndian))
	Expect(versionBigEndian.UsesMaxDataFrame()).To(BeFalse())
	Expect(versionMaxDataFrame.UsesMaxDataFrame()).To(BeTrue())
	Expect(versionBigEndian.UsesIETFFrameFormat()).To(BeFalse())
	Expect(versionIETFFrames.UsesIETFFrameFormat()).To(BeTrue())
})
