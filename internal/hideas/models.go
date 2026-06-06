package hideas

// Data model contract:
//
// The public data shapes and storage constants in this file must stay
// consistent with docs/database-design-v1.md and docs/http-api-v1.md. Node
// kind values, type domain values, JSON field names, ID semantics, timestamp
// semantics, and nullable fields are part of the documented v1.0 model.
// Changes to these structs or constants must update the relevant docs and
// tests in the same change.

const (
	KindEntity   = 1
	KindTrace    = 2
	KindRelation = 3

	DomainEntityType   = 1
	DomainTraceType    = 2
	DomainRelationType = 3
)

type Type struct {
	ID        int64  `json:"id"`
	Domain    int    `json:"domain"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Entity struct {
	ID             int64  `json:"id"`
	TypeID         *int64 `json:"type_id,omitempty"`
	TypeName       string `json:"type_name,omitempty"`
	ProfileTraceID *int64 `json:"profile_trace_id,omitempty"`
	Profile        string `json:"profile,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	Name           string `json:"name"`
}

type Trace struct {
	ID         int64  `json:"id"`
	TypeID     *int64 `json:"type_id,omitempty"`
	TypeName   string `json:"type_name,omitempty"`
	HappenedAt *int64 `json:"happened_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	Content    string `json:"content"`
}

type Relation struct {
	ID        int64  `json:"id"`
	FromKind  int    `json:"from_kind"`
	FromID    int64  `json:"from_id"`
	ToKind    int    `json:"to_kind"`
	ToID      int64  `json:"to_id"`
	TypeID    int64  `json:"type_id"`
	TypeName  string `json:"type_name,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Stats struct {
	Entities  int64 `json:"entities"`
	Traces    int64 `json:"traces"`
	Relations int64 `json:"relations"`
	Types     int64 `json:"types"`
}

type CheckResult struct {
	OK     bool     `json:"ok"`
	Errors []string `json:"errors"`
}

type DeleteResult struct {
	Kind               string  `json:"kind"`
	ID                 int64   `json:"id"`
	Cascade            bool    `json:"cascade"`
	RelationsDeleted   int     `json:"relations_deleted"`
	ProfilesCleared    int     `json:"profiles_cleared"`
	DeletedRelationIDs []int64 `json:"deleted_relation_ids,omitempty"`
}

type AddTraceInput struct {
	Content   string
	TypeName  string
	Happened  *int64
	EntityIDs []int64
	Entities  []string
}

type SearchInput struct {
	Query      string
	EntityID   *int64
	EntityName string
	TypeName   string
	Since      *int64
	Until      *int64
	Limit      int
}

type SearchResult struct {
	Traces          []Trace  `json:"traces"`
	Entities        []Entity `json:"entities"`
	TracesHasMore   bool     `json:"traces_has_more"`
	EntitiesHasMore bool     `json:"entities_has_more"`
}

type ShowResult struct {
	Kind      string     `json:"kind"`
	Entity    *Entity    `json:"entity,omitempty"`
	Trace     *Trace     `json:"trace,omitempty"`
	Relation  *Relation  `json:"relation,omitempty"`
	Entities  []Entity   `json:"entities,omitempty"`
	Traces    []Trace    `json:"traces,omitempty"`
	Relations []Relation `json:"relations,omitempty"`
}

type Store interface {
	Init() error
	Close() error
	Path() string

	AddTrace(AddTraceInput) (Trace, error)
	Search(SearchInput) (SearchResult, error)
	Show(kind string, id int64) (ShowResult, error)
	Delete(kind string, id int64, cascade bool) (DeleteResult, error)
	Link(fromKind string, fromID int64, toKind string, toID int64, typeName string) (Relation, error)

	AddEntity(name, typeName string) (Entity, error)
	ListEntities(typeName string) ([]Entity, error)
	GetEntity(id int64) (Entity, error)
	RenameEntity(id int64, name string) (Entity, error)
	ResolveEntityName(name string) ([]Entity, error)

	SetProfile(entityID int64, content string) (Trace, error)
	GetProfile(entityID int64) (Trace, error)

	ListTypes() ([]Type, error)
	AddType(domainName, name string) (Type, error)

	Stats() (Stats, error)
	Check() (CheckResult, error)
	Export(format string) ([]byte, error)
}
