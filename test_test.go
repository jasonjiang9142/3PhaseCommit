package commit

import (
	"log"
	"math/rand/v2"
	"testing"
	"time"
)

// Sends two transactions without failures.
// They should both commit and change the store
func TestBasicCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}

	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestBasicCommit: Commit when everything is fine")

	// Transaction 0
	cfg.sendSet(0, "x", 1)

	cfg.sendSet(0, "y", 2)

	cfg.sendSet(0, "z", 3)

	log.Printf("Transaction 0: Finishing")
	cfg.finishTransaction(0)
	log.Printf("Transaction 0: Finished Transaction 0")
	cfg.assertTransaction(0, true, nil)

	log.Printf("Transaction 0: Committed successfully")

	log.Printf("Transaction 1: Getting x, y, z")
	// Transaction 1
	cfg.sendGet(1, "x")
	cfg.sendGet(1, "y")
	cfg.sendGet(1, "z")

	log.Printf("Transaction 1: Finishing")

	cfg.finishTransaction(1)
	cfg.assertTransaction(1, true, map[string]interface{}{
		"x": 1,
		"y": 2,
		"z": 3,
	})

	log.Printf("Transaction 1: Read values successfully and committed")

	cfg.end()

	log.Printf("Finished TestBasicCommit")
}

// Disconnects one server before FinishTransaction
// This should cause an Abort, and the store should be unchanged
func TestBasicAbort(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}

	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestBasicAbort: Abort when a server is disconnected")

	// Successful transaction initializes values
	log.Printf("Transaction 0: Sending set x, y, z")
	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 1)
	cfg.sendSet(0, "z", 1)
	cfg.finishTransaction(0)
	log.Printf("Transaction 0: Finished Transaction 0")
	cfg.assertTransaction(0, true, map[string]interface{}{})

	// Disconnected server causes failed transaction
	log.Printf("Transaction 1: Disconnecting server 0")
	cfg.disconnect(0)

	log.Printf("Transaction 1: Sending set x, y, z")
	cfg.sendSet(1, "x", 2)
	cfg.sendSet(1, "y", 2)
	cfg.sendSet(1, "z", 2)
	cfg.finishTransaction(1)
	log.Printf("Transaction 1: Finished transaction 1. Expecting ABORT due to server disconnection")
	cfg.assertTransaction(1, false, nil)

	time.Sleep(50 * time.Millisecond) // give the servers time to finish aborting
	log.Printf("[Pause] Sleeping briefly to let abort propagate")

	// Reconnect and read old values
	log.Printf("[Recovery] Reconnecting server 0")
	cfg.connect(0)

	log.Printf("[Verify] Reading values x, y, z to confirm they are still 1")
	cfg.sendGet(2, "x")
	cfg.sendGet(2, "y")
	cfg.sendGet(2, "z")
	cfg.finishTransaction(2)
	cfg.assertTransaction(2, true, map[string]interface{}{
		"x": 1,
		"y": 1,
		"z": 1,
	})
	log.Printf("=== Finished Test: BasicAbort ===")
	cfg.end()
}

// Restarts the coordinator between transactions
// The coordinator should recover and sucessfully commit the second transaction
func TestEasyRecovery(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestEasyRecovery: Recovery between transactions")

	// Transaction 0
	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 2)
	cfg.sendSet(0, "z", 3)

	cfg.finishTransaction(0)
	cfg.assertTransaction(0, true, nil)

	// Restart coordinator
	cfg.startCoordinator()
	cfg.connectAll()

	// Transaction 1
	cfg.sendGet(1, "x")
	cfg.sendGet(1, "y")
	cfg.sendGet(1, "z")

	cfg.finishTransaction(1)
	cfg.assertTransaction(1, true, map[string]interface{}{
		"x": 1,
		"y": 2,
		"z": 3,
	})

	cfg.end()
}

// Disconnects a server that isn't relevant to the transaction after the Prepare phase is completed
// The transaction should still be able to commit
func TestRelevance(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestRelevance: Transaction can still continue if irrelevant server is disconnected after Prepare")

	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 1)
	cfg.doNextPreCommit(func() bool {
		cfg.disconnect(2)
		return true
	})
	cfg.finishTransaction(0)
	cfg.assertTransaction(0, true, nil)

	cfg.end()
}

// Sends many batches of concurrent transactions that write to separate keys
// The transactions should all be able to succeed
func TestConcurrentDifferentKeys(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestConcurrentDifferentKeys: Transactions that don't touch the same keys can always succeed concurrently")

	n := 10

	for i := range n {
		tid1 := i * 3
		tid2 := tid1 + 1
		tid3 := tid1 + 2
		cfg.sendSet(tid1, "x", i)
		cfg.sendSet(tid2, "y", i)
		cfg.sendSet(tid3, "z", i)
		cfg.finishTransaction(tid3)
		cfg.finishTransaction(tid2)
		cfg.finishTransaction(tid1)
		cfg.assertTransaction(tid3, true, nil)
		cfg.assertTransaction(tid2, true, nil)
		cfg.assertTransaction(tid1, true, nil)
	}

	cfg.end()
}

// Sends many batches of concurrent transactions that read from the same key
// The transactions should all be able to succeed
func TestConcurrentReadSameKeys(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestConcurrentReadSameKeys: Transactions that only read the same keys can always succeed concurrently")

	n := 10

	for i := range n {
		tid1 := i * 3
		tid2 := tid1 + 1
		tid3 := tid1 + 2
		key := keys[i%3][0]
		cfg.sendGet(tid1, key)
		cfg.sendGet(tid2, key)
		cfg.sendGet(tid3, key)
		cfg.finishTransaction(tid3)
		cfg.finishTransaction(tid2)
		cfg.finishTransaction(tid1)
		cfg.assertTransaction(tid3, true, nil)
		cfg.assertTransaction(tid2, true, nil)
		cfg.assertTransaction(tid1, true, nil)
	}

	cfg.end()
}

// Sends many batches of concurrent transactions that write to the same key
// At least one transaction from each batch should succeed
func TestConcurrentWriteSameKeys(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestConcurrentWriteSameKeys: Concurrent transactions that write to the same keys should have at least one commit")

	n := 10

	for i := range n {
		tid1 := i * 3
		tid2 := tid1 + 1
		tid3 := tid1 + 2
		key := keys[i%3][0]

		log.Printf("\n[Batch %d] Writing to key '%s' with transaction IDs %d, %d, %d", i, key, tid1, tid2, tid3)

		cfg.sendSet(tid1, key, i)
		cfg.sendSet(tid2, key, i)
		cfg.sendSet(tid3, key, i)

		log.Printf("[Batch %d] Finishing transactions in reverse order", i)

		cfg.finishTransaction(tid3)
		cfg.finishTransaction(tid2)
		cfg.finishTransaction(tid1)

		log.Printf("[Batch %d] Waiting for transaction results", i)

		res1 := cfg.waitTransaction(tid3)
		log.Printf("[Batch %v] res1: %v", i, res1)
		res2 := cfg.waitTransaction(tid2)
		log.Printf("[Batch %v] res2: %v", i, res2)
		res3 := cfg.waitTransaction(tid1)
		log.Printf("[Batch %v] res3: %v", i, res3)

		succCount := 0
		if res1.committed {
			log.Printf("  ✓ Transaction %d committed", tid3)
			succCount += 1
		} else {
			log.Printf("  ✗ Transaction %d aborted", tid3)
		}

		if res2.committed {
			log.Printf("  ✓ Transaction %d committed", tid2)
			succCount += 1
		} else {
			log.Printf("  ✗ Transaction %d aborted", tid2)
		}

		if res3.committed {
			log.Printf("  ✓ Transaction %d committed", tid1)
			succCount += 1
		} else {
			log.Printf("  ✗ Transaction %d aborted", tid1)
		}

		if succCount < 1 {
			t.Fatal("Not enough successes")
		} else {
			log.Printf("[Batch %d] ✅ At least one transaction committed", i)
		}

		log.Printf("--------------------------------------")

	}

	cfg.end()
}

// Sends many batches of concurrent transactions that write the same value to each key
// After each batch, the final value the keys should all be the same
func TestSerializability(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestSerializability: Concurrent transactions are executed serially")

	cfg.sendSet(0, "x", 0)
	cfg.sendSet(0, "y", 0)
	cfg.sendSet(0, "z", 0)
	cfg.finishTransaction(0)
	cfg.waitTransaction(0)

	n := 10

	oldVal := 0
	for i := range n {
		m := 3
		tidBase := i*(m+1) + 1
		for j := range m {
			tid := tidBase + j
			cfg.sendSet(tid, "x", tid)
			cfg.sendSet(tid, "y", tid)
			cfg.sendSet(tid, "z", tid)
		}

		for j := range m {
			tid := tidBase + j
			cfg.finishTransaction(tid)
		}

		for j := range m {
			tid := tidBase + j
			cfg.waitTransaction(tid)
		}

		tid := tidBase + m
		cfg.sendGet(tid, "x")
		cfg.sendGet(tid, "y")
		cfg.sendGet(tid, "z")
		cfg.finishTransaction(tid)
		resp := cfg.assertTransaction(tid, true, nil)
		x := resp.readValues["x"].(int)
		y := resp.readValues["y"].(int)
		z := resp.readValues["z"].(int)
		if x != y || x != z {
			t.Fatal("read values don't match")
		}
		if x != oldVal && x < tidBase || x > tidBase+m {
			t.Fatal("read values outside of possible range")
		}
		oldVal = x
	}

	cfg.end()
}

// Disconnects a server after the Prepare phase but before the first PreCommit goes through
// The transaction should abort
func TestDisconnectPreCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestDisconnectPreCommit: If a server disconnects before PreCommit, we abort")

	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 1)
	cfg.sendSet(0, "z", 1)
	cfg.doNextPreCommit(func() bool {
		log.Printf("Disconnecting server 0")
		cfg.disconnect(0)
		return true
	})
	log.Printf("Finishing transaction 0")
	cfg.finishTransaction(0)
	cfg.assertTransaction(0, false, nil)

	cfg.end()
}

// Disconnects a server after the PreCommit phase but before the first Commit goes through
// The transaction should block until the server is reconnected
// Then the transaction should be committed
func TestDisconnectCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestDisconnectCommit: If a server disconnects before Commit, we block until it returns")

	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 1)
	cfg.sendSet(0, "z", 1)
	cfg.doNextCommit(func() bool {
		cfg.disconnect(0)
		return true
	})
	cfg.finishTransaction(0)

	// We should be waiting for the server to return during this time
	time.Sleep(50 * time.Millisecond)
	cfg.assertNoTransaction(0)

	// Bring the server back and we commit
	cfg.connect(0)
	cfg.assertTransaction(0, true, nil)

	cfg.end()
}

// Restarts the coordinator after the Prepare phase but before the first PreCommit goes through
// The coordinator should recover and commit the transaction
func TestRestartPreCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestRestartPreCommit: If the coordinator restarts before PreCommit, we commit")

	cfg.sendSet(0, "x", 1)
	cfg.sendSet(0, "y", 1)
	cfg.sendSet(0, "z", 1)
	log.Printf("Finishing sending set 0")

	cfg.doNextPreCommit(func() bool {
		log.Printf("Restarting coordinator")
		cfg.restartCoordinatorLocked()
		return true
	})

	log.Printf("Finishing transaction 0")
	cfg.finishTransaction(0)
	log.Printf("Finished transaction 0")
	cfg.assertTransaction(0, true, nil)

	cfg.end()
}

// Restarts the coordinator after the PreCommit phase but before the first Commmit goes through
// The coordinator should recover and commit the transaction
func TestRestartCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestRestartCommit: If the coordinator restarts before Commit, we commit")

	n := 10

	for i := range n {
		cfg.sendSet(i, "x", 1)
		cfg.sendSet(i, "y", 1)
		cfg.sendSet(i, "z", 1)
		cfg.doNextCommit(func() bool {
			cfg.restartCoordinatorLocked()
			return true
		})
		cfg.finishTransaction(i)
		cfg.assertTransaction(i, true, nil)
	}

	cfg.end()
}

// Restarts the coordinator at a random time during the PreCommit phase
// The coordinator should recover and commit the transaction
// Does many trials to test different possibilities
func TestRestartMidPreCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestRestartPreCommit: If the coordinator restarts in the middle of PreCommit, we commit")

	n := 10

	for i := range n {
		cfg.sendSet(i, "x", 1)
		cfg.sendSet(i, "y", 1)
		cfg.sendSet(i, "z", 1)
		cfg.doNextPreCommit(func() bool {
			if rand.Int()%2 == 0 {
				cfg.restartCoordinatorLocked()
				return true
			}
			return false
		})
		cfg.finishTransaction(i)
		cfg.assertTransaction(i, true, nil)
	}

	cfg.end()
}

// Restarts the coordinator at a random time during the Commit phase
// The coordinator should recover and commit the transaction
// Does many trials to test different possibilities
func TestRestartMidCommit(t *testing.T) {
	keys := [][]string{
		{"x"},
		{"y"},
		{"z"},
	}
	cfg := make_config(t, keys, false, false)
	defer cfg.cleanup()

	cfg.begin("TestRestartCommit: If the coordinator restarts in the middle of Commit, we commit")

	n := 10

	for i := range n {
		cfg.sendSet(i, "x", 1)
		cfg.sendSet(i, "y", 1)
		cfg.sendSet(i, "z", 1)
		cfg.doNextCommit(func() bool {
			if rand.Int()%2 == 0 {
				cfg.restartCoordinatorLocked()
				return true
			}
			return false
		})
		cfg.finishTransaction(i)
		cfg.assertTransaction(i, true, nil)
	}

	cfg.end()
}
