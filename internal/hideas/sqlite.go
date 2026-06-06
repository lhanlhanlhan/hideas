package hideas

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var errNotFound = errors.New("not found")

type SQLiteStore struct {
	db   *sql.DB
	path string
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if path == "" {
		path = defaultDBPath()
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, path: path}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }
func (s *SQLiteStore) Path() string { return s.path }

func nowMillis() int64 { return time.Now().UTC().UnixMilli() }

func (s *SQLiteStore) Init() error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS types (
			id INTEGER PRIMARY KEY,
			domain INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(domain, name)
		)`,
		`CREATE TABLE IF NOT EXISTS entities (
			id INTEGER PRIMARY KEY,
			type_id INTEGER NULL,
			profile_trace_id INTEGER NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			name TEXT NOT NULL,
			FOREIGN KEY (type_id) REFERENCES types(id),
			FOREIGN KEY (profile_trace_id) REFERENCES traces(id)
		)`,
		`CREATE TABLE IF NOT EXISTS traces (
			id INTEGER PRIMARY KEY,
			type_id INTEGER NULL,
			happened_at INTEGER NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			content TEXT NOT NULL,
			FOREIGN KEY (type_id) REFERENCES types(id)
		)`,
		`CREATE TABLE IF NOT EXISTS relations (
			id INTEGER PRIMARY KEY,
			from_kind INTEGER NOT NULL,
			from_id INTEGER NOT NULL,
			to_kind INTEGER NOT NULL,
			to_id INTEGER NOT NULL,
			type_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY (type_id) REFERENCES types(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_type_id ON entities(type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_profile_trace_id ON entities(profile_trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_type_id ON traces(type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_happened_at ON traces(happened_at)`,
		`CREATE INDEX IF NOT EXISTS idx_relations_from_type ON relations(from_kind, from_id, type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relations_to_type ON relations(to_kind, to_id, type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relations_type_id ON relations(type_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return s.seedTypes()
}

func (s *SQLiteStore) seedTypes() error {
	seeds := map[int][]string{
		DomainEntityType:   {"person", "work", "project", "concept", "source", "place", "other"},
		DomainTraceType:    {"event", "thought", "fact", "quote", "profile", "other"},
		DomainRelationType: {"about", "mentions", "part_of", "alias_of", "derived_from", "supports", "contradicts", "related_to"},
	}
	for domain, names := range seeds {
		for _, name := range names {
			if _, err := s.ensureType(domain, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func domainFromName(name string) (int, error) {
	switch name {
	case "entity":
		return DomainEntityType, nil
	case "trace":
		return DomainTraceType, nil
	case "relation":
		return DomainRelationType, nil
	default:
		return 0, fmt.Errorf("unknown type domain: %s", name)
	}
}

func kindFromName(name string) (int, error) {
	switch name {
	case "entity":
		return KindEntity, nil
	case "trace":
		return KindTrace, nil
	case "relation":
		return KindRelation, nil
	default:
		return 0, fmt.Errorf("unknown node kind: %s", name)
	}
}

func kindName(kind int) string {
	switch kind {
	case KindEntity:
		return "entity"
	case KindTrace:
		return "trace"
	case KindRelation:
		return "relation"
	default:
		return "unknown"
	}
}

func (s *SQLiteStore) ensureType(domain int, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("type name is required")
	}
	now := nowMillis()
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO types(domain, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, domain, name, now, now); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM types WHERE domain = ? AND name = ?`, domain, name).Scan(&id)
	return id, err
}

func (s *SQLiteStore) typeIDByName(domain int, name string) (*int64, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	id, err := s.ensureType(domain, name)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func scanEntity(row interface{ Scan(...interface{}) error }) (Entity, error) {
	var e Entity
	var typeID, profile sql.NullInt64
	var typeName, profileText sql.NullString
	err := row.Scan(&e.ID, &typeID, &typeName, &profile, &profileText, &e.CreatedAt, &e.UpdatedAt, &e.Name)
	if err != nil {
		return Entity{}, err
	}
	if typeID.Valid {
		e.TypeID = &typeID.Int64
	}
	if typeName.Valid {
		e.TypeName = typeName.String
	}
	if profile.Valid {
		e.ProfileTraceID = &profile.Int64
	}
	if profileText.Valid {
		e.Profile = profileText.String
	}
	return e, nil
}

func scanTrace(row interface{ Scan(...interface{}) error }) (Trace, error) {
	var t Trace
	var typeID, happened sql.NullInt64
	var typeName sql.NullString
	err := row.Scan(&t.ID, &typeID, &typeName, &happened, &t.CreatedAt, &t.UpdatedAt, &t.Content)
	if err != nil {
		return Trace{}, err
	}
	if typeID.Valid {
		t.TypeID = &typeID.Int64
	}
	if typeName.Valid {
		t.TypeName = typeName.String
	}
	if happened.Valid {
		t.HappenedAt = &happened.Int64
	}
	return t, nil
}

func scanRelation(row interface{ Scan(...interface{}) error }) (Relation, error) {
	var r Relation
	var typeName sql.NullString
	err := row.Scan(&r.ID, &r.FromKind, &r.FromID, &r.ToKind, &r.ToID, &r.TypeID, &typeName, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Relation{}, err
	}
	if typeName.Valid {
		r.TypeName = typeName.String
	}
	return r, nil
}

const entitySelect = `SELECT e.id, e.type_id, ty.name, e.profile_trace_id, pt.content, e.created_at, e.updated_at, e.name
FROM entities e
LEFT JOIN types ty ON ty.id = e.type_id
LEFT JOIN traces pt ON pt.id = e.profile_trace_id`

const traceSelect = `SELECT tr.id, tr.type_id, ty.name, tr.happened_at, tr.created_at, tr.updated_at, tr.content
FROM traces tr
LEFT JOIN types ty ON ty.id = tr.type_id`

const relationSelect = `SELECT r.id, r.from_kind, r.from_id, r.to_kind, r.to_id, r.type_id, ty.name, r.created_at, r.updated_at
FROM relations r
LEFT JOIN types ty ON ty.id = r.type_id`

func (s *SQLiteStore) AddEntity(name, typeName string) (Entity, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Entity{}, errors.New("entity name is required")
	}
	typeID, err := s.typeIDByName(DomainEntityType, typeName)
	if err != nil {
		return Entity{}, err
	}
	now := nowMillis()
	res, err := s.db.Exec(`INSERT INTO entities(type_id, profile_trace_id, created_at, updated_at, name) VALUES (?, NULL, ?, ?, ?)`, typeID, now, now, name)
	if err != nil {
		return Entity{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetEntity(id)
}

func (s *SQLiteStore) GetEntity(id int64) (Entity, error) {
	e, err := scanEntity(s.db.QueryRow(entitySelect+` WHERE e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Entity{}, errNotFound
	}
	return e, err
}

func (s *SQLiteStore) ResolveEntityName(name string) ([]Entity, error) {
	rows, err := s.db.Query(entitySelect+` WHERE e.name = ? ORDER BY e.id`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListEntities(typeName string) ([]Entity, error) {
	q := entitySelect
	var args []interface{}
	if typeName != "" {
		typeID, err := s.typeIDByName(DomainEntityType, typeName)
		if err != nil {
			return nil, err
		}
		q += ` WHERE e.type_id = ?`
		args = append(args, *typeID)
	}
	q += ` ORDER BY e.id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RenameEntity(id int64, name string) (Entity, error) {
	if strings.TrimSpace(name) == "" {
		return Entity{}, errors.New("entity name is required")
	}
	res, err := s.db.Exec(`UPDATE entities SET name = ?, updated_at = ? WHERE id = ?`, name, nowMillis(), id)
	if err != nil {
		return Entity{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Entity{}, errNotFound
	}
	return s.GetEntity(id)
}

func (s *SQLiteStore) AddTrace(in AddTraceInput) (Trace, error) {
	if strings.TrimSpace(in.Content) == "" {
		return Trace{}, errors.New("trace content is required")
	}
	typeID, err := s.typeIDByName(DomainTraceType, in.TypeName)
	if err != nil {
		return Trace{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Trace{}, err
	}
	defer tx.Rollback()
	now := nowMillis()
	res, err := tx.Exec(`INSERT INTO traces(type_id, happened_at, created_at, updated_at, content) VALUES (?, ?, ?, ?, ?)`, typeID, in.Happened, now, now, in.Content)
	if err != nil {
		return Trace{}, err
	}
	traceID, _ := res.LastInsertId()
	var entityIDs []int64
	entityIDs = append(entityIDs, in.EntityIDs...)
	for _, name := range in.Entities {
		cands, err := s.resolveEntityNameTx(tx, name)
		if err != nil {
			return Trace{}, err
		}
		switch len(cands) {
		case 0:
			id, err := s.addEntityTx(tx, name, "")
			if err != nil {
				return Trace{}, err
			}
			entityIDs = append(entityIDs, id)
		case 1:
			entityIDs = append(entityIDs, cands[0].ID)
		default:
			return Trace{}, fmt.Errorf("ambiguous entity name: %s", name)
		}
	}
	relType, err := s.ensureTypeTx(tx, DomainRelationType, "about")
	if err != nil {
		return Trace{}, err
	}
	for _, eid := range entityIDs {
		if !s.existsTx(tx, KindEntity, eid) {
			return Trace{}, fmt.Errorf("entity not found: %d", eid)
		}
		if _, err := tx.Exec(`INSERT INTO relations(from_kind, from_id, to_kind, to_id, type_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, KindTrace, traceID, KindEntity, eid, relType, now, now); err != nil {
			return Trace{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Trace{}, err
	}
	return s.GetTrace(traceID)
}

func (s *SQLiteStore) UpdateTrace(id int64, in UpdateTraceInput) (Trace, error) {
	var sets []string
	var args []interface{}
	changed := false
	if in.HappenedAt != nil {
		sets = append(sets, "happened_at = ?")
		args = append(args, *in.HappenedAt)
		changed = true
	}
	if in.CreatedAt != nil {
		sets = append(sets, "created_at = ?")
		args = append(args, *in.CreatedAt)
		changed = true
	}
	if !changed && in.UpdatedAt == nil {
		return Trace{}, errors.New("at least one trace timestamp is required")
	}
	if in.UpdatedAt != nil {
		sets = append(sets, "updated_at = ?")
		args = append(args, *in.UpdatedAt)
	} else {
		sets = append(sets, "updated_at = ?")
		args = append(args, nowMillis())
	}
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE traces SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return Trace{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Trace{}, errNotFound
	}
	return s.GetTrace(id)
}

func (s *SQLiteStore) ensureTypeTx(tx *sql.Tx, domain int, name string) (int64, error) {
	now := nowMillis()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO types(domain, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, domain, name, now, now); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRow(`SELECT id FROM types WHERE domain = ? AND name = ?`, domain, name).Scan(&id)
	return id, err
}

func (s *SQLiteStore) addEntityTx(tx *sql.Tx, name, typeName string) (int64, error) {
	var typeID *int64
	if typeName != "" {
		id, err := s.ensureTypeTx(tx, DomainEntityType, typeName)
		if err != nil {
			return 0, err
		}
		typeID = &id
	}
	now := nowMillis()
	res, err := tx.Exec(`INSERT INTO entities(type_id, profile_trace_id, created_at, updated_at, name) VALUES (?, NULL, ?, ?, ?)`, typeID, now, now, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) resolveEntityNameTx(tx *sql.Tx, name string) ([]Entity, error) {
	rows, err := tx.Query(entitySelect+` WHERE e.name = ? ORDER BY e.id`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) existsTx(tx *sql.Tx, kind int, id int64) bool {
	var n int
	table := map[int]string{KindEntity: "entities", KindTrace: "traces", KindRelation: "relations"}[kind]
	if table == "" {
		return false
	}
	_ = tx.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n)
	return n > 0
}

func (s *SQLiteStore) GetTrace(id int64) (Trace, error) {
	t, err := scanTrace(s.db.QueryRow(traceSelect+` WHERE tr.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Trace{}, errNotFound
	}
	return t, err
}

func (s *SQLiteStore) GetRelation(id int64) (Relation, error) {
	r, err := scanRelation(s.db.QueryRow(relationSelect+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Relation{}, errNotFound
	}
	return r, err
}

func (s *SQLiteStore) Search(in SearchInput) (SearchResult, error) {
	if in.Limit < 0 {
		in.Limit = 0
	}
	var entityID *int64
	if in.EntityID != nil {
		entityID = in.EntityID
	} else if in.EntityName != "" {
		cands, err := s.ResolveEntityName(in.EntityName)
		if err != nil {
			return SearchResult{}, err
		}
		if len(cands) == 0 {
			return SearchResult{}, nil
		}
		if len(cands) > 1 {
			return SearchResult{}, fmt.Errorf("ambiguous entity name: %s", in.EntityName)
		}
		entityID = &cands[0].ID
	}
	q := traceSelect
	var where []string
	var args []interface{}
	if in.Query != "" {
		where = append(where, `tr.content LIKE ?`)
		args = append(args, "%"+in.Query+"%")
	}
	if in.TypeName != "" {
		typeID, err := s.typeIDByName(DomainTraceType, in.TypeName)
		if err != nil {
			return SearchResult{}, err
		}
		where = append(where, `tr.type_id = ?`)
		args = append(args, *typeID)
	}
	if in.Since != nil {
		where = append(where, `COALESCE(tr.happened_at, tr.created_at) >= ?`)
		args = append(args, *in.Since)
	}
	if in.Until != nil {
		where = append(where, `COALESCE(tr.happened_at, tr.created_at) <= ?`)
		args = append(args, *in.Until)
	}
	if entityID != nil {
		q += ` JOIN relations er ON er.from_kind = 2 AND er.from_id = tr.id AND er.to_kind = 1 AND er.to_id = ?`
		args = append([]interface{}{*entityID}, args...)
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY COALESCE(tr.happened_at, tr.created_at) DESC, tr.id DESC LIMIT ?`
	args = append(args, in.Limit+1)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()
	var res SearchResult
	for rows.Next() {
		t, err := scanTrace(rows)
		if err != nil {
			return SearchResult{}, err
		}
		res.Traces = append(res.Traces, t)
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, err
	}
	if len(res.Traces) > in.Limit {
		res.TracesHasMore = true
		res.Traces = res.Traces[:in.Limit]
	}
	if in.Query != "" {
		es, more, err := s.searchEntities(in.Query, in.Limit)
		if err != nil {
			return SearchResult{}, err
		}
		res.Entities = es
		res.EntitiesHasMore = more
	}
	return res, nil
}

func (s *SQLiteStore) searchEntities(query string, limit int) ([]Entity, bool, error) {
	rows, err := s.db.Query(entitySelect+` WHERE e.name LIKE ? ORDER BY e.id LIMIT ?`, "%"+query+"%", limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func (s *SQLiteStore) Show(kind string, id int64) (ShowResult, error) {
	switch kind {
	case "entity":
		e, err := s.GetEntity(id)
		if err != nil {
			return ShowResult{}, err
		}
		traces, err := s.tracesForEntity(id)
		if err != nil {
			return ShowResult{}, err
		}
		rels, err := s.relationsFor(KindEntity, id)
		if err != nil {
			return ShowResult{}, err
		}
		return ShowResult{Kind: kind, Entity: &e, Traces: traces, Relations: rels}, nil
	case "trace":
		t, err := s.GetTrace(id)
		if err != nil {
			return ShowResult{}, err
		}
		entities, err := s.entitiesForTrace(id)
		if err != nil {
			return ShowResult{}, err
		}
		rels, err := s.relationsFor(KindTrace, id)
		if err != nil {
			return ShowResult{}, err
		}
		return ShowResult{Kind: kind, Trace: &t, Entities: entities, Relations: rels}, nil
	case "relation":
		r, err := s.GetRelation(id)
		if err != nil {
			return ShowResult{}, err
		}
		rels, err := s.relationsFor(KindRelation, id)
		if err != nil {
			return ShowResult{}, err
		}
		return ShowResult{Kind: kind, Relation: &r, Relations: rels}, nil
	default:
		return ShowResult{}, fmt.Errorf("unknown kind: %s", kind)
	}
}

func (s *SQLiteStore) Delete(kindName string, id int64, cascade bool) (DeleteResult, error) {
	kind, err := kindFromName(kindName)
	if err != nil {
		return DeleteResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DeleteResult{}, err
	}
	defer tx.Rollback()
	if !s.existsTx(tx, kind, id) {
		return DeleteResult{}, fmt.Errorf("%s not found: %d", kindName, id)
	}
	blockers, err := s.deleteBlockersTx(tx, kind, id)
	if err != nil {
		return DeleteResult{}, err
	}
	if len(blockers) > 0 && !cascade {
		return DeleteResult{}, fmt.Errorf("delete blocked: %s; use --cascade to delete related relations and clear profile references", strings.Join(blockers, "; "))
	}
	res := DeleteResult{Kind: kindName, ID: id, Cascade: cascade}
	var relationIDs []int64
	if cascade {
		relationIDs, err = s.collectCascadeRelationIDsTx(tx, kind, id)
		if err != nil {
			return DeleteResult{}, err
		}
		if kind == KindTrace {
			n, err := s.clearProfileReferencesTx(tx, id)
			if err != nil {
				return DeleteResult{}, err
			}
			res.ProfilesCleared = n
		}
	}
	if kind == KindRelation {
		relationIDs = append(relationIDs, id)
	}
	relationIDs = uniqueInt64s(relationIDs)
	if len(relationIDs) > 0 {
		if err := s.deleteRelationsTx(tx, relationIDs); err != nil {
			return DeleteResult{}, err
		}
		res.RelationsDeleted = len(relationIDs)
		res.DeletedRelationIDs = relationIDs
	}
	if kind != KindRelation {
		table := map[int]string{KindEntity: "entities", KindTrace: "traces"}[kind]
		r, err := tx.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
		if err != nil {
			return DeleteResult{}, err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return DeleteResult{}, errNotFound
		}
	}
	if err := tx.Commit(); err != nil {
		return DeleteResult{}, err
	}
	return res, nil
}

func (s *SQLiteStore) deleteBlockersTx(tx *sql.Tx, kind int, id int64) ([]string, error) {
	var blockers []string
	rows, err := tx.Query(`SELECT id FROM relations WHERE (from_kind = ? AND from_id = ?) OR (to_kind = ? AND to_id = ?) ORDER BY id`, kind, id, kind, id)
	if err != nil {
		return nil, err
	}
	var relIDs []string
	for rows.Next() {
		var relID int64
		if err := rows.Scan(&relID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		relIDs = append(relIDs, strconv.FormatInt(relID, 10))
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(relIDs) > 0 {
		blockers = append(blockers, "referenced by relations "+strings.Join(relIDs, ","))
	}
	if kind == KindTrace {
		rows, err := tx.Query(`SELECT id FROM entities WHERE profile_trace_id = ? ORDER BY id`, id)
		if err != nil {
			return nil, err
		}
		var entityIDs []string
		for rows.Next() {
			var entityID int64
			if err := rows.Scan(&entityID); err != nil {
				_ = rows.Close()
				return nil, err
			}
			entityIDs = append(entityIDs, strconv.FormatInt(entityID, 10))
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(entityIDs) > 0 {
			blockers = append(blockers, "used as profile by entities "+strings.Join(entityIDs, ","))
		}
	}
	return blockers, nil
}

func (s *SQLiteStore) collectCascadeRelationIDsTx(tx *sql.Tx, kind int, id int64) ([]int64, error) {
	seen := map[int64]bool{}
	var queue []int64
	if kind == KindRelation {
		queue = append(queue, id)
	}
	direct, err := s.relationIDsReferencingTx(tx, kind, id)
	if err != nil {
		return nil, err
	}
	queue = append(queue, direct...)
	for len(queue) > 0 {
		relID := queue[0]
		queue = queue[1:]
		if seen[relID] {
			continue
		}
		seen[relID] = true
		next, err := s.relationIDsReferencingTx(tx, KindRelation, relID)
		if err != nil {
			return nil, err
		}
		queue = append(queue, next...)
	}
	var ids []int64
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (s *SQLiteStore) relationIDsReferencingTx(tx *sql.Tx, kind int, id int64) ([]int64, error) {
	rows, err := tx.Query(`SELECT id FROM relations WHERE (from_kind = ? AND from_id = ?) OR (to_kind = ? AND to_id = ?) ORDER BY id`, kind, id, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var relID int64
		if err := rows.Scan(&relID); err != nil {
			return nil, err
		}
		ids = append(ids, relID)
	}
	return ids, rows.Err()
}

func (s *SQLiteStore) clearProfileReferencesTx(tx *sql.Tx, traceID int64) (int, error) {
	r, err := tx.Exec(`UPDATE entities SET profile_trace_id = NULL, updated_at = ? WHERE profile_trace_id = ?`, nowMillis(), traceID)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) deleteRelationsTx(tx *sql.Tx, relationIDs []int64) error {
	for _, id := range relationIDs {
		if _, err := tx.Exec(`DELETE FROM relations WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func uniqueInt64s(in []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *SQLiteStore) tracesForEntity(id int64) ([]Trace, error) {
	rows, err := s.db.Query(traceSelect+` JOIN relations r ON r.from_kind = 2 AND r.from_id = tr.id AND r.to_kind = 1 AND r.to_id = ? ORDER BY tr.id DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trace
	for rows.Next() {
		t, err := scanTrace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) entitiesForTrace(id int64) ([]Entity, error) {
	rows, err := s.db.Query(entitySelect+` JOIN relations r ON r.to_kind = 1 AND r.to_id = e.id AND r.from_kind = 2 AND r.from_id = ? ORDER BY e.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) relationsFor(kind int, id int64) ([]Relation, error) {
	rows, err := s.db.Query(relationSelect+` WHERE (r.from_kind = ? AND r.from_id = ?) OR (r.to_kind = ? AND r.to_id = ?) ORDER BY r.id`, kind, id, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relation
	for rows.Next() {
		r, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Link(fromKind string, fromID int64, toKind string, toID int64, typeName string) (Relation, error) {
	fk, err := kindFromName(fromKind)
	if err != nil {
		return Relation{}, err
	}
	tk, err := kindFromName(toKind)
	if err != nil {
		return Relation{}, err
	}
	typeID, err := s.ensureType(DomainRelationType, typeName)
	if err != nil {
		return Relation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Relation{}, err
	}
	defer tx.Rollback()
	if !s.existsTx(tx, fk, fromID) {
		return Relation{}, fmt.Errorf("%s not found: %d", fromKind, fromID)
	}
	if !s.existsTx(tx, tk, toID) {
		return Relation{}, fmt.Errorf("%s not found: %d", toKind, toID)
	}
	now := nowMillis()
	res, err := tx.Exec(`INSERT INTO relations(from_kind, from_id, to_kind, to_id, type_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, fk, fromID, tk, toID, typeID, now, now)
	if err != nil {
		return Relation{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return Relation{}, err
	}
	return s.GetRelation(id)
}

func (s *SQLiteStore) SetProfile(entityID int64, content string) (Trace, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Trace{}, err
	}
	defer tx.Rollback()
	if !s.existsTx(tx, KindEntity, entityID) {
		return Trace{}, fmt.Errorf("entity not found: %d", entityID)
	}
	typeID, err := s.ensureTypeTx(tx, DomainTraceType, "profile")
	if err != nil {
		return Trace{}, err
	}
	now := nowMillis()
	res, err := tx.Exec(`INSERT INTO traces(type_id, happened_at, created_at, updated_at, content) VALUES (?, NULL, ?, ?, ?)`, typeID, now, now, content)
	if err != nil {
		return Trace{}, err
	}
	traceID, _ := res.LastInsertId()
	if _, err := tx.Exec(`UPDATE entities SET profile_trace_id = ?, updated_at = ? WHERE id = ?`, traceID, now, entityID); err != nil {
		return Trace{}, err
	}
	relType, err := s.ensureTypeTx(tx, DomainRelationType, "about")
	if err != nil {
		return Trace{}, err
	}
	if _, err := tx.Exec(`INSERT INTO relations(from_kind, from_id, to_kind, to_id, type_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, KindTrace, traceID, KindEntity, entityID, relType, now, now); err != nil {
		return Trace{}, err
	}
	if err := tx.Commit(); err != nil {
		return Trace{}, err
	}
	return s.GetTrace(traceID)
}

func (s *SQLiteStore) GetProfile(entityID int64) (Trace, error) {
	var traceID sql.NullInt64
	err := s.db.QueryRow(`SELECT profile_trace_id FROM entities WHERE id = ?`, entityID).Scan(&traceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Trace{}, errNotFound
	}
	if err != nil {
		return Trace{}, err
	}
	if !traceID.Valid {
		return Trace{}, errNotFound
	}
	return s.GetTrace(traceID.Int64)
}

func (s *SQLiteStore) ListTypes() ([]Type, error) {
	rows, err := s.db.Query(`SELECT id, domain, name, created_at, updated_at FROM types ORDER BY domain, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Type
	for rows.Next() {
		var t Type
		if err := rows.Scan(&t.ID, &t.Domain, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) AddType(domainName, name string) (Type, error) {
	domain, err := domainFromName(domainName)
	if err != nil {
		return Type{}, err
	}
	id, err := s.ensureType(domain, name)
	if err != nil {
		return Type{}, err
	}
	var t Type
	err = s.db.QueryRow(`SELECT id, domain, name, created_at, updated_at FROM types WHERE id = ?`, id).Scan(&t.ID, &t.Domain, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *SQLiteStore) Stats() (Stats, error) {
	var st Stats
	for table, dest := range map[string]*int64{"entities": &st.Entities, "traces": &st.Traces, "relations": &st.Relations, "types": &st.Types} {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(dest); err != nil {
			return Stats{}, err
		}
	}
	return st, nil
}

func (s *SQLiteStore) Check() (CheckResult, error) {
	rows, err := s.db.Query(`SELECT id, from_kind, from_id, to_kind, to_id FROM relations ORDER BY id`)
	if err != nil {
		return CheckResult{}, err
	}
	type endpointRelation struct {
		id, fromID, toID int64
		fromKind, toKind int
	}
	var rels []endpointRelation
	res := CheckResult{OK: true}
	for rows.Next() {
		var r endpointRelation
		if err := rows.Scan(&r.id, &r.fromKind, &r.fromID, &r.toKind, &r.toID); err != nil {
			_ = rows.Close()
			return CheckResult{}, err
		}
		rels = append(rels, r)
	}
	if err := rows.Close(); err != nil {
		return CheckResult{}, err
	}
	if err := rows.Err(); err != nil {
		return CheckResult{}, err
	}
	for _, r := range rels {
		if !s.exists(r.fromKind, r.fromID) {
			res.OK = false
			res.Errors = append(res.Errors, fmt.Sprintf("relation %d has missing from %s %d", r.id, kindName(r.fromKind), r.fromID))
		}
		if !s.exists(r.toKind, r.toID) {
			res.OK = false
			res.Errors = append(res.Errors, fmt.Sprintf("relation %d has missing to %s %d", r.id, kindName(r.toKind), r.toID))
		}
	}
	return res, nil
}

func (s *SQLiteStore) exists(kind int, id int64) bool {
	var n int
	table := map[int]string{KindEntity: "entities", KindTrace: "traces", KindRelation: "relations"}[kind]
	if table == "" {
		return false
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n)
	return n > 0
}

func (s *SQLiteStore) Export(format string) ([]byte, error) {
	types, err := s.ListTypes()
	if err != nil {
		return nil, err
	}
	entities, err := s.ListEntities("")
	if err != nil {
		return nil, err
	}
	traces, err := s.allTraces()
	if err != nil {
		return nil, err
	}
	relations, err := s.allRelations()
	if err != nil {
		return nil, err
	}
	data := map[string]interface{}{"types": types, "entities": entities, "traces": traces, "relations": relations}
	switch format {
	case "", "json":
		return json.MarshalIndent(data, "", "  ")
	case "markdown":
		var b strings.Builder
		b.WriteString("# hideas export\n\n## Entities\n\n")
		for _, e := range entities {
			fmt.Fprintf(&b, "- %d %s [%s]\n", e.ID, e.Name, e.TypeName)
		}
		b.WriteString("\n## Traces\n\n")
		for _, t := range traces {
			fmt.Fprintf(&b, "- %d %s\n", t.ID, t.Content)
		}
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("unknown export format: %s", format)
	}
}

func (s *SQLiteStore) allTraces() ([]Trace, error) {
	rows, err := s.db.Query(traceSelect + ` ORDER BY tr.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trace
	for rows.Next() {
		t, err := scanTrace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) allRelations() ([]Relation, error) {
	rows, err := s.db.Query(relationSelect + ` ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relation
	for rows.Next() {
		r, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func defaultDBPath() string {
	switch runtime.GOOS {
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, "Library", "Application Support", "hideas", "hideas.sqlite")
		}
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "hideas", "hideas.sqlite")
		}
	default:
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			return filepath.Join(dataHome, "hideas", "hideas.sqlite")
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, ".local", "share", "hideas", "hideas.sqlite")
		}
	}
	return "hideas.sqlite"
}
