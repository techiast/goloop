package service

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/icon-project/goloop/common/codec"

	"github.com/icon-project/goloop/module"
)

type serviceReactor struct {
	membership module.Membership
	txPool     *transactionPool
}

const (
	reactorName           = "serviceReactor"
	PROPAGATE_TRANSACTION = protocolInfo(0x1001)
)

var (
	sReactorCodec = codec.MP
	subProtocols  = []module.ProtocolInfo{PROPAGATE_TRANSACTION}
)

func (r *serviceReactor) OnReceive(subProtocol module.ProtocolInfo, buf []byte, peerId module.PeerID) (bool, error) {
	switch subProtocol {
	case PROPAGATE_TRANSACTION:
		var tx transaction
		if _, err := sReactorCodec.UnmarshalFromBytes(buf, &tx); err != nil {
			log.Printf("Failed to unmarshal transaction. buf = %x, err = %s\n", buf, err)
		}

		if err := tx.Verify(); err != nil {
			log.Printf("Failed to verify tx. err = %x\n", err)
			return false, err
		}
		if err := r.txPool.add(&tx); err != nil {
			log.Printf("Failed to add tx. tx = %v, err = %s\n", tx, err)
		}
		return true, nil
	}
	return false, nil
}

func (r *serviceReactor) propagateTransaction(pi module.ProtocolInfo, tx *transaction) error {
	buf, err := sReactorCodec.MarshalToBytes(tx)
	if err != nil {
		log.Printf("Failed to marshal transaction. tx = %v, err = %s\n", tx, err)
	}

	r.membership.Multicast(PROPAGATE_TRANSACTION, buf, module.ROLE_VALIDATOR)
	return nil
}

func (r *serviceReactor) OnError() {
}

type protocolInfo uint16

func (pi protocolInfo) ID() byte {
	return byte(pi >> 8)
}

func (pi protocolInfo) Version() byte {
	return byte(pi)
}

func (pi protocolInfo) Copy(b []byte) {
	binary.BigEndian.PutUint16(b[:2], uint16(pi))
}

func (pi protocolInfo) String() string {
	return fmt.Sprintf("{ID:SERVICE:%#02x,Ver:%#02x}", pi.ID(), pi.Version())
}

func (pi protocolInfo) Uint16() uint16 {
	return uint16(pi)
}
