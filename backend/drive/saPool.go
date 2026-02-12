// Service Account Pool for eclone
//
// Merges gclone's SaInfo (sequential rollup, stale tracking) with fclone's
// ServiceAccountPool (preloaded services, 25h blacklist, random selection).
//
// Key improvements over gclone:
//   - Service preloading eliminates 200-500ms switch latency
//   - 25h blacklist (sync.Map) aligns with Google's daily quota reset
//   - Preloaded drive.Service pool for instant SA switches
//
// Key improvements over fclone:
//   - Bug fix: _getFile blacklists the correct file (not empty string)
//   - os.ReadDir replaces deprecated ioutil.ReadDir
//   - os.ReadFile replaces deprecated ioutil.ReadFile
//   - env.ShellExpand replaces os.ExpandEnv for consistency
//   - Retains gclone's rollup() for proactive rolling SA rotation
package drive

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/lib/env"
	drive "google.golang.org/api/drive/v3"
)

// serviceAccountBlacklist tracks SA files that hit rate limits.
// Keys are file paths (string), values are time.Time of when they were blacklisted.
// Entries expire after 25 hours, aligning with Google's daily quota reset.
var serviceAccountBlacklist sync.Map

const blacklistDuration = 25 * time.Hour

// SaEntry represents a single service account file with its stale state.
// The isStale flag is used by rollup() to skip exhausted SAs during sequential rotation.
type SaEntry struct {
	saPath  string
	isStale bool
}

// ServiceAccountInfo holds a pre-created Drive service and its HTTP client,
// ready for immediate use without the ~200-500ms OAuth setup latency.
type ServiceAccountInfo struct {
	Service *drive.Service
	Client  *http.Client
}

// ServiceAccountPool manages service account files and preloaded services.
//
// It combines two rotation strategies:
//   - Sequential rollup (from gclone): cycles through SAs in order via rollup()
//   - Random selection with blacklist (from fclone): picks random SA, skipping
//     recently-exhausted ones via GetFile()
//
// The pool also maintains a slice of pre-created ServiceAccountInfo for instant
// SA switches without OAuth setup overhead.
type ServiceAccountPool struct {
	// --- From gclone: sequential rollup support ---
	sas       map[int]SaEntry  // indexed SA entries for rollup
	activeIdx int              // current active index in sas
	saPool    map[string]int   // reverse lookup: path → index

	// --- From fclone: preloaded services + file pool ---
	ctx   context.Context
	Files map[string]struct{} // available SA file paths (for GetFile)
	Max   int                 // max preloaded services to keep
	svcs  []ServiceAccountInfo
	mu    *sync.Mutex
}

// NewServiceAccountPool creates a new empty pool.
// max controls how many preloaded services to keep in memory.
func NewServiceAccountPool(ctx context.Context, max int) *ServiceAccountPool {
	return &ServiceAccountPool{
		sas:    make(map[int]SaEntry),
		saPool: make(map[string]int),
		ctx:    ctx,
		Files:  make(map[string]struct{}),
		Max:    max,
		mu:     new(sync.Mutex),
	}
}

// =====================================================================
// gclone-compatible methods (sequential rollup, stale tracking)
// =====================================================================

// updateSas initializes the SA index from a list of file paths.
// If activeSa is not in the list, it gets appended.
func (p *ServiceAccountPool) updateSas(data []string, activeSa string) {
	if len(data) == 0 || activeSa == "" {
		return
	}
	convSas := make(map[int]SaEntry)
	convData := make(map[string]int)

	for i, v := range data {
		convSas[i] = SaEntry{saPath: v, isStale: false}
		convData[v] = i
	}
	p.sas = convSas
	p.saPool = convData

	if result := p.findIdxByStrInPool(activeSa); result != -1 {
		p.activeIdx = result
	} else {
		existLen := len(p.sas)
		p.sas[existLen] = SaEntry{saPath: activeSa, isStale: false}
		p.saPool[activeSa] = existLen
		p.activeIdx = existLen
	}
}

func (p *ServiceAccountPool) findIdxByStrInPool(str string) int {
	if idx, ok := p.saPool[str]; ok {
		return idx
	}
	return -1
}

func (p *ServiceAccountPool) findIdxByStr(str string) int {
	for k, v := range p.sas {
		if v.saPath == str {
			return k
		}
	}
	return -1
}

// rollup returns the next non-stale SA file path in sequential order,
// wrapping around from the end to the beginning. Returns "" if all SAs are stale.
// This is gclone's unique proactive rotation feature — it switches SA
// before each operation rather than waiting for rate limit errors.
func (p *ServiceAccountPool) rollup() string {
	existLen := len(p.sas)
	// Search forward from activeIdx+1
	for i := p.activeIdx + 1; i < existLen; i++ {
		if !p.sas[i].isStale {
			return p.sas[i].saPath
		}
	}
	// Wrap around from 0 to activeIdx
	for i := 0; i < p.activeIdx; i++ {
		if !p.sas[i].isStale {
			return p.sas[i].saPath
		}
	}
	return ""
}

// activeSa sets the active index to the given SA path.
func (p *ServiceAccountPool) activeSa(saPath string) {
	if entry, ok := p.saPool[saPath]; ok {
		p.activeIdx = entry
	}
}

// staleSa marks the given SA (or current active if target=="") as stale,
// removes it from the pool, and picks a new random SA.
// Returns (true, "") if no SAs remain, or (false, newPath) on success.
func (p *ServiceAccountPool) staleSa(target string) (bool, string) {
	if target == "" {
		target = p.sas[p.activeIdx].saPath
	}
	oldIdx := p.saPool[target]
	if entry, ok := p.sas[oldIdx]; ok {
		entry.isStale = true
		p.sas[oldIdx] = entry
	}
	delete(p.saPool, target)
	if p.isPoolEmpty() {
		p.activeIdx = -1
		return true, ""
	}
	if ret := p.randomPick(); ret != -1 {
		p.activeIdx = ret
		return false, p.sas[ret].saPath
	}
	return true, ""
}

// randomPick selects a random index from the non-stale SA pool.
func (p *ServiceAccountPool) randomPick() int {
	existLen := len(p.saPool)
	if existLen == 0 {
		return -1
	}

	r := rand.Intn(existLen)
	for _, v := range p.saPool {
		if r == 0 {
			return v
		}
		r--
	}
	return -1
}

// isPoolEmpty returns true if no non-stale SAs remain.
func (p *ServiceAccountPool) isPoolEmpty() bool {
	return len(p.saPool) == 0
}

// revertStaleSa un-stales a previously staled SA, returning it to the pool.
func (p *ServiceAccountPool) revertStaleSa(target string) {
	if target == "" {
		return
	}
	if oldIdx := p.findIdxByStr(target); oldIdx != -1 {
		if entry, ok := p.sas[oldIdx]; ok {
			entry.isStale = false
			p.saPool[target] = oldIdx
			p.sas[oldIdx] = entry
		}
	}
}

// =====================================================================
// fclone-compatible methods (file pool, preloaded services, blacklist)
// =====================================================================

// Load reads .json SA files from the configured ServiceAccountFilePath directory,
// populating both the Files map (for GetFile/blacklist) and the sas/saPool maps
// (for rollup/staleSa). The activeSa file is excluded from the Files map but
// included in the sas index.
func (p *ServiceAccountPool) Load(opt *Options) (map[string]struct{}, error) {
	saFolder := opt.ServiceAccountFilePath
	if saFolder == "" {
		return p.Files, nil
	}

	fs.Debugf(nil, "Loading Service Account File(s) from %q", saFolder)
	entries, err := os.ReadDir(saFolder)
	if err != nil {
		return nil, fmt.Errorf("error loading service accounts from folder: %w", err)
	}

	fileList := make(map[string]struct{})
	var fileNames []string

	pathSeparator := string(os.PathSeparator)
	if !strings.HasSuffix(saFolder, pathSeparator) {
		saFolder += pathSeparator
	}

	for _, entry := range entries {
		filePath := fmt.Sprintf("%s%s", saFolder, entry.Name())
		if path.Ext(filePath) != ".json" {
			continue
		}
		fileNames = append(fileNames, filePath)
		// Exclude the currently active SA from the file pool
		// (it's already in use, no need to pick it again)
		if filePath != opt.ServiceAccountFile {
			fileList[filePath] = struct{}{}
		}
	}

	p.Files = fileList
	p.updateSas(fileNames, opt.ServiceAccountFile)

	fs.Debugf(nil, "Loaded %d Service Account File(s)", len(fileList))
	return fileList, nil
}

// AddService pushes a service to the front of the preloaded pool.
// If the pool exceeds Max, the oldest entry is dropped.
func (p *ServiceAccountPool) AddService(client *http.Client, svc *drive.Service) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.svcs = append([]ServiceAccountInfo{{Service: svc, Client: client}}, p.svcs...)
	if len(p.svcs) > p.Max {
		p.svcs = p.svcs[:p.Max]
	}
}

// GetService returns a preloaded service from the front and rotates it to the back.
func (p *ServiceAccountPool) GetService() (*drive.Service, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.svcs) == 0 {
		return nil, fmt.Errorf("no available preloaded services")
	}
	svc := p.svcs[0].Service
	p.svcs = append(p.svcs[1:], p.svcs[0])
	return svc, nil
}

// GetClient returns a preloaded HTTP client from the front and rotates it to the back.
func (p *ServiceAccountPool) GetClient() (*http.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.svcs) == 0 {
		return nil, fmt.Errorf("no available preloaded services")
	}
	client := p.svcs[0].Client
	p.svcs = append(p.svcs[1:], p.svcs[0])
	return client, nil
}

// PreloadServices creates Drive services from SA files and adds them to the pool.
// This eliminates the 200-500ms OAuth setup latency during SA switches.
func (p *ServiceAccountPool) PreloadServices(f *Fs, count int) ([]ServiceAccountInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var svcs []ServiceAccountInfo
	for file := range p.Files {
		if len(svcs) >= count {
			break
		}
		svc, err := createDriveService(p.ctx, &f.opt, file)
		if err != nil {
			fs.Errorf(nil, "Preloading Service Account (%s): %v", file, err)
			continue
		}
		svcs = append(svcs, svc)
	}

	p.svcs = append(svcs, p.svcs...)
	fs.Debugf(nil, "Preloaded %d Service(s) from Service Account", len(svcs))
	return svcs, nil
}

// GetFile returns a random SA file path from the pool, skipping blacklisted ones.
// If excludeFile is non-empty, that file is blacklisted and removed from the pool
// before selection (typically the currently-failing SA).
//
// NOTE: This fixes a bug in fclone where serviceAccountBlacklist.Store was called
// with an empty string because the file variable hadn't been assigned yet.
func (p *ServiceAccountPool) GetFile(excludeFile string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p._getFile(excludeFile)
}

func (p *ServiceAccountPool) _getFile(excludeFile string) (string, error) {
	// Blacklist and remove the excluded file first
	if excludeFile != "" {
		serviceAccountBlacklist.Store(excludeFile, time.Now())
		delete(p.Files, excludeFile)
	}

	if len(p.Files) == 0 {
		return "", fmt.Errorf("no available service account file")
	}

	// Collect available keys
	keys := make([]string, 0, len(p.Files))
	for k := range p.Files {
		keys = append(keys, k)
	}

	// Random permutation, pick first non-blacklisted file
	perm := rand.Perm(len(keys))
	for _, idx := range perm {
		file := keys[idx]
		blackTime, ok := serviceAccountBlacklist.Load(file)
		if !ok || time.Since(blackTime.(time.Time)) > blacklistDuration {
			// Not blacklisted or blacklist expired — clear and use
			if ok {
				serviceAccountBlacklist.Delete(file)
			}
			return file, nil
		}
	}

	return "", fmt.Errorf("no available service account file (all blacklisted)")
}

// =====================================================================
// Helper: create a Drive service from a SA file
// =====================================================================

// createDriveService reads a SA credentials file and creates a Drive service.
// Uses getServiceAccountClient() from drive.go for OAuth client creation.
func createDriveService(ctx context.Context, opt *Options, file string) (svc ServiceAccountInfo, err error) {
	loadedCreds, err := os.ReadFile(env.ShellExpand(file))
	if err != nil {
		err = fmt.Errorf("error opening service account credentials file: %w", err)
		return
	}
	svc.Client, err = getServiceAccountClient(ctx, opt, loadedCreds)
	if err != nil {
		err = fmt.Errorf("failed to create oauth client from service account: %w", err)
		return
	}
	svc.Service, err = drive.New(svc.Client)
	if err != nil {
		err = fmt.Errorf("couldn't create Drive client: %w", err)
		return
	}
	return
}
