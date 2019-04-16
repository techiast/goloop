package service

import (
	"testing"
	"time"

	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/common/db"
	"github.com/icon-project/goloop/common/errors"
)

type mockMonitor struct {
}

func (m *mockMonitor) OnDropTx(bs int) {
	// do nothing
}

func (m *mockMonitor) OnAddTx(bs int) {
	// do nothing
}

func (m *mockMonitor) OnRemoveTx(bs int) {
	// do nothing
}

func (m *mockMonitor) OnCommit(id []byte, ts time.Time, d time.Duration) {
	// do nothing
}

func TestTransactionPool_Add(t *testing.T) {
	dbase := db.NewMapDB()
	bk, _ := dbase.GetBucket(db.TransactionLocatorByHash)
	pool := NewTransactionPool(1, bk, &mockMonitor{})

	addr := common.NewAddressFromString("hx1111111111111111111111111111111111111111")
	tx1 := newMockTransaction([]byte("tx1"), addr, 1)
	tx1.NID = 1

	if err := pool.Add(tx1, true); err != nil {
		t.Error("Fail to add transaction with valid network ID")
	}

	tx2 := newMockTransaction([]byte("tx2"), addr, 2)
	tx2.NID = 2

	if err := pool.Add(tx2, true); err == nil {
		t.Error("It should fail to add transaction with different NID")
	} else {
		if c := errors.CodeOf(err); c != errors.InvalidNetworkError {
			t.Errorf(
				"Expected error code is InvalidNetworkError code=%d", c)
		}
	}
}
