package seeder

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	rolerepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
)

// workflowSeedContext carries only fixture-owned identities. Child seeders must
// verify existing operational rows and preserve state changed through the APIs.
type workflowSeedContext struct {
	Context         context.Context
	DB              *gorm.DB
	Now             time.Time
	Anchor          time.Time
	Location        *time.Location
	Users           map[string]string
	Hospitals       map[string]string
	IDs             map[string]string
	ContractStorage storage.Client
	MedicalStorage  storage.Client
	BaseURL         string
}

func workflowUUID(key string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("medikaone:demo:workflow:v1:"+key)).String()
}

// RunWithWorkflows adds usable, PDF-backed scenarios without rewinding existing
// appointments, clinical revisions, or invitation decisions on subsequent runs.
func RunWithWorkflows(ctx context.Context, db *gorm.DB, contracts, medical storage.Client, now time.Time, baseURL string) (*DemoManifest, error) {
	return runWithWorkflows(ctx, db, contracts, medical, now, baseURL, false)
}

// ResetAllAndSeedWithWorkflows is reserved for the existing explicitly confirmed
// full-reset command. It retains the allowlisted, transactional reset boundary.
func ResetAllAndSeedWithWorkflows(ctx context.Context, db *gorm.DB, contracts, medical storage.Client, now time.Time, baseURL string) (*DemoManifest, error) {
	return runWithWorkflows(ctx, db, contracts, medical, now, baseURL, true)
}

func runWithWorkflows(ctx context.Context, db *gorm.DB, contracts, medical storage.Client, now time.Time, baseURL string, reset bool) (*DemoManifest, error) {
	if contracts == nil || medical == nil {
		return nil, errors.New("workflow seed requires working contract and medical object storage")
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("workflow seed requires an absolute HTTP(S) SERVER_BASE_URL without credentials, query, or fragment")
	}
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, fmt.Errorf("load demo timezone: %w", err)
	}
	if now.IsZero() {
		now = time.Now()
	}
	trackedContracts := &demoUploadTracker{client: contracts}
	trackedMedical := &demoUploadTracker{client: medical}
	var manifest *DemoManifest
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('medikaone:demo:workflow:v1', 0))`).Error; err != nil {
			return fmt.Errorf("lock workflow fixtures: %w", err)
		}
		if reset {
			if err := tx.Exec(resetAllDataSQL).Error; err != nil {
				return fmt.Errorf("truncate application data: %w", err)
			}
		}
		if err := seedAll(tx); err != nil {
			return err
		}
		state, err := newWorkflowSeedContext(ctx, tx, trackedContracts, trackedMedical, now, strings.TrimRight(parsed.String(), "/"), location)
		if err != nil {
			return err
		}
		// Base fixtures deliberately scope staff roles to a hospital. Doctors
		// also need the self-service role used by mobile /v1/doctor endpoints.
		roles := rolerepo.NewRepository(tx)
		doctorRoleID, err := roles.GetRoleIDBySlug(ctx, constant.RoleDoctor)
		if err != nil {
			return fmt.Errorf("resolve mobile doctor role: %w", err)
		}
		for _, actor := range []string{"doctor1", "doctor2", "doctor3"} {
			if err := roles.Assign(ctx, state.Users[actor], doctorRoleID); err != nil {
				return fmt.Errorf("assign mobile role to %s: %w", actor, err)
			}
		}
		if err := seedDemoPractices(state); err != nil {
			return fmt.Errorf("seed demo practices: %w", err)
		}
		if err := seedDemoClinical(state); err != nil {
			return fmt.Errorf("seed demo clinical workflows: %w", err)
		}
		manifest, err = buildDemoManifest(state)
		return err
	})
	if err != nil {
		// The database transaction rolls back, but uploads do not. Remove only
		// successful uploads from this invocation, never pre-existing documents.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, trackedContracts.cleanup(cleanupCtx), trackedMedical.cleanup(cleanupCtx))
	}
	fmt.Println("[seeder] workflow fixtures ready; reruns preserve clinical progress and original scenario dates")
	return manifest, nil
}

func newWorkflowSeedContext(ctx context.Context, db *gorm.DB, contracts, medical storage.Client, now time.Time, baseURL string, location *time.Location) (*workflowSeedContext, error) {
	state := &workflowSeedContext{
		Context: ctx, DB: db, Now: now, Anchor: now, Location: location,
		Users: make(map[string]string, 11), Hospitals: make(map[string]string, 2), IDs: make(map[string]string),
		ContractStorage: contracts, MedicalStorage: medical, BaseURL: baseURL,
	}
	for _, actor := range []struct{ alias, email string }{
		{"superadmin", "superadmin@medikaone.id"}, {"admin", "admin001@medikaone.id"},
		{"nurse", "nurse001@medikaone.id"}, {"receptionist", "receptionist001@medikaone.id"}, {"bod", "bod001@medikaone.id"},
		{"doctor1", "doctor001@medikaone.id"}, {"doctor2", "doctor002@medikaone.id"}, {"doctor3", "doctor003@medikaone.id"},
		{"patient1", "patient001@medikaone.id"}, {"patient2", "patient002@medikaone.id"}, {"patient3", "patient003@medikaone.id"},
	} {
		id, err := getUserIDBySeedKey(db, demoUserSeedKey(actor.email))
		if err != nil {
			return nil, err
		}
		state.Users[actor.alias] = id
	}
	for _, hospital := range []struct{ alias, key string }{
		{"jakarta", "medikaone:hospital:general-jakarta"}, {"bandung", "medikaone:hospital:clinic-bandung"},
	} {
		id, err := getHospitalIDBySeedKey(db, hospital.key)
		if err != nil {
			return nil, err
		}
		state.Hospitals[hospital.alias] = id
	}
	return state, nil
}

// Tracking is limited to newly uploaded objects; successful service-level
// cleanup removes the path from the rollback set.
type demoUploadTracker struct {
	client storage.Client
	paths  []string
}

func (tracker *demoUploadTracker) Upload(ctx context.Context, path, contentType string, content []byte) (*storage.UploadedObject, error) {
	object, err := tracker.client.Upload(ctx, path, contentType, content)
	if err == nil && object != nil {
		tracker.paths = append(tracker.paths, object.ObjectPath)
	}
	return object, err
}

func (tracker *demoUploadTracker) Delete(ctx context.Context, path string) error {
	if err := tracker.client.Delete(ctx, path); err != nil {
		return err
	}
	for i, uploaded := range tracker.paths {
		if uploaded == path {
			tracker.paths[i] = tracker.paths[len(tracker.paths)-1]
			tracker.paths = tracker.paths[:len(tracker.paths)-1]
			break
		}
	}
	return nil
}

func (tracker *demoUploadTracker) CreateSignedURL(ctx context.Context, path string, ttl time.Duration, filename string) (string, error) {
	return tracker.client.CreateSignedURL(ctx, path, ttl, filename)
}

func (tracker *demoUploadTracker) cleanup(ctx context.Context) error {
	var failures []error
	for _, path := range tracker.paths {
		if err := tracker.client.Delete(ctx, path); err != nil {
			failures = append(failures, fmt.Errorf("clean up newly uploaded demo object %s: %w", path, err))
		}
	}
	return errors.Join(failures...)
}
