package drill

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shanurwan/failure-drill/internal/kube"
)

// Mongo says where the analytics database is and how to talk to it. Tyk Pump
// moves the gateway's analytics records out of Redis into this database, one
// document per request, so counting documents says how much of the traffic the
// system still has a record of.
type Mongo struct {
	Selector   string
	Container  string
	DB         string
	Collection string
	// ShellArgs are extra mongosh / mongodump / mongorestore arguments, such
	// as credentials.
	ShellArgs []string
}

// eval runs JavaScript with mongosh inside a pod, against that pod's own mongod.
func (m Mongo) eval(ctx context.Context, k *kube.Client, pod string, timeout time.Duration, js string) (string, error) {
	args := append([]string{"mongosh", "--quiet"}, m.ShellArgs...)
	out, err := k.Exec(ctx, pod, m.Container, timeout, append(args, "--eval", js)...)
	return strings.TrimSpace(out), err
}

// roles asks every pod what it is: "primary", "secondary", or "" when it does
// not answer in time.
func (m Mongo) roles(ctx context.Context, k *kube.Client) (map[string]string, error) {
	pods, err := k.Pods(ctx, m.Selector)
	if err != nil {
		return nil, err
	}
	type answer struct{ pod, role string }
	answers := make(chan answer, len(pods))
	for _, pod := range pods {
		go func(name string, running bool) {
			role := ""
			if running {
				out, err := m.eval(ctx, k, name, 2*time.Second,
					`const h = db.hello(); print(h.isWritablePrimary ? "primary" : h.secondary ? "secondary" : "other")`)
				if err == nil {
					role = out
				}
			}
			answers <- answer{name, role}
		}(pod.Name, pod.DeletionTimestamp == nil && pod.Status.Phase == "Running")
	}
	roles := make(map[string]string, len(pods))
	for range pods {
		a := <-answers
		roles[a.pod] = a.role
	}
	return roles, nil
}

func (m Mongo) primary(ctx context.Context, k *kube.Client) (string, map[string]string, error) {
	roles, err := m.roles(ctx, k)
	if err != nil {
		return "", nil, err
	}
	if len(roles) == 0 {
		return "", nil, fmt.Errorf("no pods match --mongo-selector %q in namespace %s", m.Selector, k.Namespace)
	}
	var primaries []string
	for pod, role := range roles {
		if role == "primary" {
			primaries = append(primaries, pod)
		}
	}
	if len(primaries) != 1 {
		return "", nil, fmt.Errorf("expected exactly one MongoDB primary, found %d (%s)", len(primaries), describe(roles))
	}
	return primaries[0], roles, nil
}

// count returns how many documents in the collection have a timestamp in
// [from, to). A zero time leaves that side open.
func (m Mongo) count(ctx context.Context, k *kube.Client, pod string, from, to time.Time) (int, error) {
	var bounds []string
	if !from.IsZero() {
		bounds = append(bounds, fmt.Sprintf("$gte: new Date(%d)", from.UnixMilli()))
	}
	if !to.IsZero() {
		bounds = append(bounds, fmt.Sprintf("$lt: new Date(%d)", to.UnixMilli()))
	}
	filter := "{}"
	if len(bounds) > 0 {
		filter = fmt.Sprintf("{timestamp: {%s}}", strings.Join(bounds, ", "))
	}
	out, err := m.eval(ctx, k, pod, 10*time.Second,
		fmt.Sprintf(`print(db.getSiblingDB(%q).getCollection(%q).countDocuments(%s))`, m.DB, m.Collection, filter))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("count documents on %s: unexpected answer %q", pod, out)
	}
	return n, nil
}

// account waits for the pump to finish writing, then compares the documents
// recorded during the drill with the requests the gateway answered.
func (m Mongo) account(ctx context.Context, k *kube.Client, traffic Traffic, note func(kind, text string), fact func(key, value string)) {
	// The pump writes in batches every few seconds: wait until the count
	// has stopped moving.
	var recorded, stable int
	for attempt := 0; attempt < 45 && stable < 4; attempt++ {
		if !sleep(ctx, time.Second) {
			return
		}
		primary, _, err := m.primary(ctx, k)
		if err != nil {
			continue
		}
		n, err := m.count(ctx, k, primary, traffic.From, traffic.To.Add(2*time.Second))
		if err != nil {
			continue
		}
		if n == recorded {
			stable++
		} else {
			recorded, stable = n, 0
		}
	}
	fact("requests_answered", strconv.Itoa(traffic.Answered))
	fact("analytics_recorded", strconv.Itoa(recorded))
	missing := traffic.Answered - recorded
	fact("analytics_missing", strconv.Itoa(max(missing, 0)))
	switch {
	case missing > 0:
		note("analytics", fmt.Sprintf("the gateway answered %d requests; MongoDB has a record of %d: %d missing", traffic.Answered, recorded, missing))
	default:
		note("analytics", fmt.Sprintf("the gateway answered %d requests; MongoDB has a record of %d: none missing", traffic.Answered, recorded))
	}
}

// MongoPrimary takes the MongoDB primary away.
//
//   - crash: the pod is killed without warning and the replica set elects
//     another member.
//   - stepdown: the primary is asked to step down, as happens in every planned
//     upgrade of a replica set.
type MongoPrimary struct {
	Mongo
	Mode string

	primary    string
	last       map[string]string
	injectedAt time.Time
	elected    bool
}

func (m *MongoPrimary) Name() string { return "mongo-primary" }
func (m *MongoPrimary) Kind() string { return m.Mode }

func (m *MongoPrimary) Prepare(ctx context.Context, k *kube.Client) (string, error) {
	if m.Mode != "crash" && m.Mode != "stepdown" {
		return "", fmt.Errorf("unknown --fault %q for mongo-primary: use crash or stepdown", m.Mode)
	}
	primary, roles, err := m.Mongo.primary(ctx, k)
	if err != nil {
		return "", err
	}
	if len(roles) < 2 {
		return "", fmt.Errorf("%s is the only MongoDB member: there is nobody to elect", primary)
	}
	m.primary, m.last = primary, roles
	if m.Mode == "crash" {
		return fmt.Sprintf("kill %s (primary of %d members) without warning", primary, len(roles)), nil
	}
	return fmt.Sprintf("ask %s (primary of %d members) to step down for 60s", primary, len(roles)), nil
}

func (m *MongoPrimary) Inject(ctx context.Context, k *kube.Client) error {
	m.injectedAt = time.Now()
	if m.Mode == "crash" {
		zero := int64(0)
		return k.DeletePod(ctx, m.primary, &zero)
	}
	// Stepping down closes the connection the command arrived on, so an
	// error here is the expected outcome and the watcher confirms the result.
	_, _ = m.eval(ctx, k, m.primary, 15*time.Second, `try { rs.stepDown(60) } catch (e) { print(e.codeName || e.message) }`)
	return nil
}

func (m *MongoPrimary) Watch(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string)) {
	roles, err := m.roles(ctx, k)
	if err != nil || ctx.Err() != nil {
		return
	}
	fact("primary_before", m.primary)
	pods := make([]string, 0, len(roles))
	for pod := range roles {
		pods = append(pods, pod)
	}
	sort.Strings(pods)
	for _, pod := range pods {
		role, was := roles[pod], m.last[pod]
		if role == was {
			continue
		}
		switch role {
		case "primary":
			note("mongo", fmt.Sprintf("%s elected primary", pod))
			if !m.elected && !m.injectedAt.IsZero() && pod != m.primary {
				m.elected = true
				fact("primary_after", pod)
				fact("election_s", fmt.Sprintf("%.1f", time.Since(m.injectedAt).Seconds()))
			}
		case "secondary":
			if was == "primary" {
				note("mongo", fmt.Sprintf("%s stepped down, now a secondary", pod))
			} else {
				note("mongo", fmt.Sprintf("%s is back as a secondary", pod))
			}
		case "":
			note("mongo", fmt.Sprintf("%s stopped answering", pod))
		default:
			note("mongo", fmt.Sprintf("%s is starting up", pod))
		}
	}
	m.last = roles
}

func (m *MongoPrimary) Account(ctx context.Context, k *kube.Client, traffic Traffic, note func(kind, text string), fact func(key, value string)) {
	m.account(ctx, k, traffic, note, fact)
}

// MongoRestore rehearses losing the analytics database and getting it back.
//
// It takes a backup with mongodump, lets traffic continue for Gap so that
// there is data newer than the backup, drops the database, and restores the
// backup with mongorestore. It reports how long the restore took and how many
// records were written after the backup and are therefore gone: the two
// numbers a backup policy promises and seldom measures.
type MongoRestore struct {
	Mongo
	Gap time.Duration
	// ArchivePath is where the backup is kept on disk before anything is
	// dropped, so that an interrupted drill can be finished by hand.
	ArchivePath string

	primary string
}

func (m *MongoRestore) Name() string { return "mongo-restore" }
func (m *MongoRestore) Kind() string { return "drop-and-restore" }

func (m *MongoRestore) Prepare(ctx context.Context, k *kube.Client) (string, error) {
	primary, _, err := m.Mongo.primary(ctx, k)
	if err != nil {
		return "", err
	}
	m.primary = primary
	total, err := m.count(ctx, k, primary, time.Time{}, time.Time{})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("back up database %q (%d records in %s), wait %s, drop it, restore it on %s",
		m.DB, total, m.Collection, m.Gap, primary), nil
}

func (m *MongoRestore) Inject(context.Context, *kube.Client) error { return nil }

func (m *MongoRestore) Watch(context.Context, *kube.Client, func(kind, text string), func(key, value string)) {
}

// mongorestore reports each collection on standard error; newer versions put
// the namespace in backquotes.
var restoredLine = regexp.MustCompile("finished restoring `?([^` ]+)`? \\((\\d+) documents?, (\\d+) failures?\\)")

func (m *MongoRestore) After(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string)) error {
	tool := func(name string, args ...string) []string {
		return append(append([]string{name}, m.ShellArgs...), args...)
	}

	// 1. The backup.
	backupStarted := time.Now()
	var archive bytes.Buffer
	if _, err := k.ExecIO(ctx, m.primary, m.Container, 10*time.Minute, nil, &archive,
		tool("mongodump", "--archive", "--gzip", "--db", m.DB)...); err != nil {
		return fmt.Errorf("back up %s: %w", m.DB, err)
	}
	backupTook := time.Since(backupStarted)
	fact("backup_s", fmt.Sprintf("%.1f", backupTook.Seconds()))
	fact("backup_bytes", strconv.Itoa(archive.Len()))
	if err := os.WriteFile(m.ArchivePath, archive.Bytes(), 0o600); err != nil {
		return fmt.Errorf("keep the backup on disk before dropping anything: %w", err)
	}
	note("backup", fmt.Sprintf("backup taken: %d bytes in %.1fs, kept at %s", archive.Len(), backupTook.Seconds(), m.ArchivePath))

	// 2. Life goes on: everything written from here is newer than the backup.
	if !sleep(ctx, m.Gap) {
		return ctx.Err()
	}

	// 3. The disaster. Records old enough to be safely in the backup are
	// counted first, so the restore can be checked against them.
	settled := backupStarted.Add(-5 * time.Second)
	oldBefore, err := m.count(ctx, k, m.primary, time.Time{}, settled)
	if err != nil {
		return err
	}
	atDisaster, err := m.count(ctx, k, m.primary, time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	dropped := time.Now()
	if out, err := m.eval(ctx, k, m.primary, 30*time.Second,
		fmt.Sprintf(`print(db.getSiblingDB(%q).dropDatabase().ok)`, m.DB)); err != nil || out != "1" {
		return fmt.Errorf("drop database %s: answered %q: %v", m.DB, out, err)
	}
	fact("records_at_disaster", strconv.Itoa(atDisaster))
	note("fault", fmt.Sprintf("database %s dropped with %d records in %s", m.DB, atDisaster, m.Collection))

	// 4. The restore. No --drop: the pump is still writing, and what it has
	// written since the disaster should survive.
	report, err := k.ExecIO(ctx, m.primary, m.Container, 15*time.Minute, bytes.NewReader(archive.Bytes()), nil,
		tool("mongorestore", "--archive", "--gzip", "--nsInclude", m.DB+".*")...)
	if err != nil {
		return fmt.Errorf("restore %s: %w", m.DB, err)
	}
	restoreTook := time.Since(dropped)
	restored := -1
	for _, match := range restoredLine.FindAllStringSubmatch(report, -1) {
		if match[1] == m.DB+"."+m.Collection {
			restored, _ = strconv.Atoi(match[2])
		}
	}
	if restored < 0 {
		return fmt.Errorf("mongorestore did not report restoring %s.%s:\n%s", m.DB, m.Collection, report)
	}
	fact("restore_s", fmt.Sprintf("%.1f", restoreTook.Seconds()))
	fact("records_restored", strconv.Itoa(restored))
	note("restore", fmt.Sprintf("restore finished: %d records back, %.1fs after the drop", restored, restoreTook.Seconds()))

	// 5. The verdict.
	lost := atDisaster - restored
	fact("records_lost", strconv.Itoa(max(lost, 0)))
	fact("lost_window_s", fmt.Sprintf("%.1f", dropped.Sub(backupStarted).Seconds()))
	note("restore", fmt.Sprintf("%d records written in the %.1fs between the backup and the disaster are gone", max(lost, 0), dropped.Sub(backupStarted).Seconds()))

	oldAfter, err := m.count(ctx, k, m.primary, time.Time{}, settled)
	if err != nil {
		return err
	}
	if oldAfter == oldBefore {
		fact("verified", "yes")
		note("restore", fmt.Sprintf("verified: all %d records older than the backup are back", oldAfter))
	} else {
		fact("verified", "no")
		note("restore", fmt.Sprintf("NOT verified: %d records older than the backup before the drop, %d after the restore", oldBefore, oldAfter))
	}
	return nil
}
