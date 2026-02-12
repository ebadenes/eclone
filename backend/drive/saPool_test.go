package drive

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// =====================================================================
// gclone-compatible tests (ported from saInfo_test.go)
// =====================================================================

func newTestPool() *ServiceAccountPool {
	return NewServiceAccountPool(context.Background(), 100)
}

func TestUpdate(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d"}

	a.updateSas(b, "a")
	assert.Equal(t, 0, a.activeIdx)
	a.updateSas(b, "d")
	assert.Equal(t, 3, a.activeIdx)
	a.updateSas(b, "e")
	assert.Equal(t, 4, a.activeIdx)
}

func TestActive(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d"}
	a.updateSas(b, "a")

	a.activeSa("c")
	assert.Equal(t, 2, a.activeIdx)
	a.activeSa("f")
	assert.Equal(t, 2, a.activeIdx)
}

func TestStale(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d"}
	a.updateSas(b, "a")

	err, newOne := a.staleSa("")
	assert.Equal(t, false, err)
	assert.NotEqual(t, "a", newOne)
	assert.Equal(t, 3, len(a.saPool))
	assert.Equal(t, 4, len(a.sas))

	a.activeSa(newOne)
	assert.NotEqual(t, 0, a.activeIdx)

	err, newOne = a.staleSa("")
	assert.Equal(t, false, err)
	assert.Equal(t, 2, len(a.saPool))
	a.activeSa(newOne)
}

func TestStaleEnd(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b"}
	a.updateSas(b, "a")

	err, newOne := a.staleSa("")
	assert.Equal(t, false, err)
	assert.NotEqual(t, "a", newOne)
	assert.Equal(t, 1, len(a.saPool))
	assert.Equal(t, true, a.sas[0].isStale)
	a.activeSa(newOne)

	err, newOne = a.staleSa("")
	assert.Equal(t, true, err)
	assert.Equal(t, "", newOne)
}

func TestRollingDirect(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c"}
	a.updateSas(b, "a")

	nextSa := a.rollup()
	assert.Equal(t, "b", nextSa)
	a.activeSa(nextSa)
	assert.Equal(t, 1, a.activeIdx)

	nextSa = a.rollup()
	assert.Equal(t, "c", nextSa)
	a.activeSa(nextSa)
	assert.Equal(t, 2, a.activeIdx)

	// Wraps around to "a"
	nextSa = a.rollup()
	assert.Equal(t, "a", nextSa)
	a.activeSa(nextSa)
	assert.Equal(t, 0, a.activeIdx)
}

func TestRollingWithStale(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d"}
	a.updateSas(b, "a")

	// Stale "a", get a new random one
	err, newOne := a.staleSa("")
	assert.Equal(t, false, err)
	a.activeSa(newOne)
	assert.NotEqual(t, "a", newOne)

	// Rolling should skip stale "a"
	nextSa := a.rollup()
	a.activeSa(nextSa)
	assert.NotEqual(t, 0, a.activeIdx)

	nextSa = a.rollup()
	idx := a.saPool[nextSa]
	a.activeSa(nextSa)
	assert.NotEqual(t, 0, a.activeIdx)

	err, newOne = a.staleSa("")
	assert.Equal(t, false, err)
	a.activeSa(newOne)
	assert.NotEqual(t, "a", newOne)

	nextSa = a.rollup()
	assert.NotEqual(t, 0, a.activeIdx)
	assert.NotEqual(t, idx, a.activeIdx)
	a.activeSa(nextSa)

	nextSa = a.rollup()
	a.activeSa(nextSa)
	assert.NotEqual(t, 0, a.activeIdx)
	assert.NotEqual(t, idx, a.activeIdx)
	idx = a.saPool[nextSa]

	err, newOne = a.staleSa("")
	assert.Equal(t, false, err)
	a.activeSa(newOne)
	assert.NotEqual(t, "a", newOne)

	nextSa = a.rollup()
	a.activeSa(nextSa)
	assert.NotEqual(t, 0, a.activeIdx)
	assert.NotEqual(t, idx, a.activeIdx)
}

func TestEmptyInit(t *testing.T) {
	a := newTestPool()
	b := []string{}
	a.updateSas(b, "")

	assert.Equal(t, true, a.isPoolEmpty())
}

func TestRevertStaleSa(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d"}
	a.updateSas(b, "a")

	_, step2Sa := a.staleSa("")
	a.activeSa(step2Sa)
	step2Idx := a.activeIdx

	assert.NotEqual(t, 0, a.activeIdx)
	assert.Equal(t, step2Sa, a.sas[a.activeIdx].saPath)

	_, step3Sa := a.staleSa("")
	a.activeSa(step3Sa)
	assert.NotEqual(t, step2Idx, a.activeIdx)
	assert.Equal(t, step3Sa, a.sas[a.activeIdx].saPath)
	assert.Equal(t, true, a.sas[0].isStale)
	assert.Equal(t, true, a.sas[step2Idx].isStale)

	a.revertStaleSa("a")
	assert.Equal(t, false, a.sas[0].isStale)
	assert.Equal(t, true, a.sas[step2Idx].isStale)

	// Reverting non-existent SA should be safe
	a.revertStaleSa("f")
	assert.Equal(t, false, a.sas[0].isStale)
	assert.Equal(t, true, a.sas[step2Idx].isStale)
}

func TestRandomPick(t *testing.T) {
	a := newTestPool()
	b := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	a.updateSas(b, "b")

	// Just verify it returns valid indices
	for i := 0; i < 10; i++ {
		idx := a.randomPick()
		assert.True(t, idx >= 0 && idx < len(b), "randomPick returned out of range: %d", idx)
	}
}

// =====================================================================
// New tests for fclone-ported features
// =====================================================================

func TestNewServiceAccountPool(t *testing.T) {
	pool := NewServiceAccountPool(context.Background(), 50)
	assert.NotNil(t, pool)
	assert.Equal(t, 50, pool.Max)
	assert.NotNil(t, pool.Files)
	assert.NotNil(t, pool.sas)
	assert.NotNil(t, pool.saPool)
	assert.NotNil(t, pool.mu)
}

func TestGetFileExclude(t *testing.T) {
	pool := newTestPool()
	pool.Files = map[string]struct{}{
		"/sa/sa1.json": {},
		"/sa/sa2.json": {},
		"/sa/sa3.json": {},
	}

	// Clear any leftover blacklist entries from other tests
	serviceAccountBlacklist.Range(func(key, value interface{}) bool {
		serviceAccountBlacklist.Delete(key)
		return true
	})

	// GetFile excluding sa1 should blacklist sa1 and return sa2 or sa3
	file, err := pool.GetFile("/sa/sa1.json")
	assert.NoError(t, err)
	assert.NotEqual(t, "/sa/sa1.json", file)
	assert.Contains(t, []string{"/sa/sa2.json", "/sa/sa3.json"}, file)

	// sa1 should be removed from Files
	_, exists := pool.Files["/sa/sa1.json"]
	assert.False(t, exists)

	// sa1 should be blacklisted
	_, blacklisted := serviceAccountBlacklist.Load("/sa/sa1.json")
	assert.True(t, blacklisted)

	// Clean up
	serviceAccountBlacklist.Delete("/sa/sa1.json")
}

func TestGetFileEmpty(t *testing.T) {
	pool := newTestPool()
	pool.Files = map[string]struct{}{}

	_, err := pool.GetFile("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available service account file")
}

func TestGetFileAllBlacklisted(t *testing.T) {
	pool := newTestPool()
	pool.Files = map[string]struct{}{
		"/sa/sa1.json": {},
		"/sa/sa2.json": {},
	}

	// Blacklist all files
	serviceAccountBlacklist.Store("/sa/sa1.json", time.Now())
	serviceAccountBlacklist.Store("/sa/sa2.json", time.Now())

	_, err := pool.GetFile("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all blacklisted")

	// Clean up
	serviceAccountBlacklist.Delete("/sa/sa1.json")
	serviceAccountBlacklist.Delete("/sa/sa2.json")
}

func TestBlacklistExpiry(t *testing.T) {
	pool := newTestPool()
	pool.Files = map[string]struct{}{
		"/sa/sa1.json": {},
	}

	// Blacklist sa1 with a time far in the past (expired)
	serviceAccountBlacklist.Store("/sa/sa1.json", time.Now().Add(-26*time.Hour))

	// Should still return sa1 because blacklist expired (>25h)
	file, err := pool.GetFile("")
	assert.NoError(t, err)
	assert.Equal(t, "/sa/sa1.json", file)

	// Blacklist entry should have been cleaned up
	_, stillBlacklisted := serviceAccountBlacklist.Load("/sa/sa1.json")
	assert.False(t, stillBlacklisted)
}

func TestAddAndGetService(t *testing.T) {
	pool := newTestPool()
	pool.Max = 3

	// Add 2 services (using nil for real Service/Client — only testing pool logic)
	pool.AddService(nil, nil)
	pool.AddService(nil, nil)
	assert.Equal(t, 2, len(pool.svcs))

	// GetService should work
	svc, err := pool.GetService()
	assert.NoError(t, err)
	assert.Nil(t, svc) // we added nil services

	// Pool should still have 2 (rotated, not removed)
	assert.Equal(t, 2, len(pool.svcs))
}

func TestGetServiceEmpty(t *testing.T) {
	pool := newTestPool()

	_, err := pool.GetService()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available preloaded services")

	_, err = pool.GetClient()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available preloaded services")
}

func TestAddServiceMaxCap(t *testing.T) {
	pool := newTestPool()
	pool.Max = 2

	pool.AddService(nil, nil)
	pool.AddService(nil, nil)
	pool.AddService(nil, nil) // should be capped at Max=2

	assert.Equal(t, 2, len(pool.svcs))
}

func TestGetFileNoExclude(t *testing.T) {
	pool := newTestPool()
	pool.Files = map[string]struct{}{
		"/sa/sa1.json": {},
		"/sa/sa2.json": {},
	}

	// Clear blacklist
	serviceAccountBlacklist.Range(func(key, value interface{}) bool {
		serviceAccountBlacklist.Delete(key)
		return true
	})

	// GetFile with empty excludeFile should not blacklist anything
	file, err := pool.GetFile("")
	assert.NoError(t, err)
	assert.Contains(t, []string{"/sa/sa1.json", "/sa/sa2.json"}, file)

	// Both files should still be in the pool
	assert.Equal(t, 2, len(pool.Files))
}

func TestGetFileBugFix(t *testing.T) {
	// This test verifies the fclone bug fix:
	// In fclone, _getFile(true) would call serviceAccountBlacklist.Store(file, ...)
	// BEFORE file was assigned, blacklisting empty string instead of the actual file.
	// Our fix: GetFile takes excludeFile string parameter explicitly.
	pool := newTestPool()
	pool.Files = map[string]struct{}{
		"/sa/sa1.json": {},
		"/sa/sa2.json": {},
		"/sa/sa3.json": {},
	}

	// Clear blacklist
	serviceAccountBlacklist.Range(func(key, value interface{}) bool {
		serviceAccountBlacklist.Delete(key)
		return true
	})

	// Exclude sa2 — it should be blacklisted, not ""
	file, err := pool.GetFile("/sa/sa2.json")
	assert.NoError(t, err)
	assert.NotEqual(t, "/sa/sa2.json", file)

	// Verify sa2 is blacklisted (not empty string)
	_, sa2Blacklisted := serviceAccountBlacklist.Load("/sa/sa2.json")
	assert.True(t, sa2Blacklisted, "sa2 should be blacklisted")

	// Verify empty string is NOT blacklisted (the bug)
	_, emptyBlacklisted := serviceAccountBlacklist.Load("")
	assert.False(t, emptyBlacklisted, "empty string should NOT be blacklisted (fclone bug)")

	// Clean up
	serviceAccountBlacklist.Delete("/sa/sa2.json")
}

func TestConcurrentGetFile(t *testing.T) {
	pool := newTestPool()
	for i := 0; i < 20; i++ {
		pool.Files[fmt.Sprintf("/sa/sa%d.json", i)] = struct{}{}
	}

	// Clear blacklist
	serviceAccountBlacklist.Range(func(key, value interface{}) bool {
		serviceAccountBlacklist.Delete(key)
		return true
	})

	// Concurrent access should not panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.GetFile("")
		}()
	}
	wg.Wait()
}
