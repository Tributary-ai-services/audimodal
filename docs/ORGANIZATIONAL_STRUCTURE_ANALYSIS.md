# Organizations and Tenant Structure Analysis

## Platform Organizational Models

### **Aether (Frontend/Neo4j) - Multi-Tier Organizational Structure**

**Hierarchical Structure**:
```
User
  ↓
Organizations (Company/Business Units)
  ↓  
Teams (Departments/Groups)
  ↓
Notebooks (Projects/Content)
  ↓
Documents (Files)
```

**Organization Model** (`organization.go`):
- **Core Fields**: `ID`, `Name`, `Slug`, `Description`, `Visibility` (public/private)
- **Membership**: `MemberCount`, `TeamCount`, `NotebookCount`
- **Roles**: owner, admin, member, billing
- **Business Features**: Billing info, settings, website, location
- **Permissions**: Role-based access control with granular permissions

**Team Model** (`team.go`):
- **Core Fields**: `ID`, `Name`, `Description`, `OrganizationID`, `Visibility`
- **Membership**: `MemberCount` with roles (owner, admin, member, viewer)
- **Scope**: Organization-bound entities
- **Visibility**: private, organization, public

**Key Relationships**:
- Organizations contain multiple teams
- Teams contain multiple users with different roles
- Notebooks can be owned by organizations, teams, or individual users
- Complex permission inheritance and visibility controls

### **AudiModal (Processing Engine/PostgreSQL) - Single-Tier Tenant Model**

**Structure**:
```
Tenant (Single Level)
  ↓
DataSources
  ↓
Files
  ↓
Chunks
```

**Tenant Model** (`tenant.go`):
- **Core Fields**: `ID`, `Name`, `DisplayName`, `BillingPlan`, `BillingEmail`
- **Resource Quotas**: FilesPerHour, StorageGB, ComputeHours, APIRequestsPerMinute, MaxConcurrentJobs
- **Compliance**: GDPR, HIPAA, SOX, PCI flags, data residency, retention policies
- **Contact Info**: AdminEmail, SecurityEmail, BillingEmail, TechnicalEmail
- **No Sub-Organizations**: Flat structure with no teams or sub-tenants

**Data Isolation**:
- All models have `TenantID` for strict data isolation
- Files, DataSources, ProcessingSessions, DLPPolicies all tenant-scoped
- Quota enforcement at tenant level

### **DeepLake API (Vector Storage/Python) - Basic Tenant Support**

**Structure**:
```
Tenant (Optional/Basic)
  ↓
Datasets
  ↓
Vectors
```

**Tenant Support** (from `auth_service.py`):
- **Simple Model**: Basic tenant ID in auth tokens and API keys
- **Default Tenant**: 'default' tenant for development
- **Minimal Features**: tenant_id in datasets and vectors, basic quotas
- **No Organization Structure**: No teams, roles, or complex hierarchies

## Critical Organizational Structure Inconsistencies

### **1. Structural Mismatch: Multi-Tier vs Single-Tier**

**Issue**: Fundamental architectural difference in organizational models

**Aether**: Rich 3-tier hierarchy (Organization → Team → User)
```go
type Organization struct {
    ID             string
    Name           string
    Visibility     string  // public/private
    MemberCount    int
    TeamCount      int
    NotebookCount  int
    // Complex billing, settings, permissions
}

type Team struct {
    ID             string
    OrganizationID string
    Visibility     string  // private/organization/public
    MemberCount    int
    // Team-specific settings and permissions
}
```

**AudiModal**: Flat single-tier tenant model
```go
type Tenant struct {
    ID           uuid.UUID
    Name         string
    DisplayName  string
    BillingPlan  string
    // No sub-organizations or teams
    // All entities directly tenant-scoped
}
```

**DeepLake**: Minimal tenant concept
```python
# Basic tenant_id in tokens/datasets
# No organizational structure
```

### **2. Permission Model Conflicts**

**Aether**: Complex role-based permissions
- Organization roles: owner, admin, member, billing
- Team roles: owner, admin, member, viewer
- Notebook visibility: private, shared, public
- Permission inheritance from organization → team → notebook

**AudiModal**: Simple tenant isolation
- All-or-nothing tenant access
- No role differentiation within tenant
- Binary access: tenant member or not

**DeepLake**: Basic API key permissions
- Simple permission list: ['read', 'write', 'admin']
- No organizational context in permissions

### **3. Data Ownership and Sharing Issues**

**Current Flow Problems**:

1. **Team Collaboration Loss**: 
   - Aether: User creates notebook in Team A under Organization X
   - AudiModal: Creates DataSource with single TenantID (loses team context)
   - DeepLake: Vectors have tenant_id but no team/organization context
   - **Result**: Team collaboration features don't work in processed data

2. **Cross-Organization Sharing**:
   - Aether: Public notebooks can be shared across organizations
   - AudiModal: Strict tenant isolation prevents cross-tenant sharing
   - **Result**: Public sharing doesn't work in vector search

3. **Permission Enforcement**:
   - Search results in DeepLake have no organization/team context
   - Cannot enforce Aether's complex visibility rules in vector search
   - Team members might see vectors from notebooks they shouldn't access

### **4. User Identity Mapping Issues**

**Complex ID Resolution Chain**:
```
Keycloak User ID → Aether Internal User ID → Organization/Team Membership → AudiModal Tenant ID → DeepLake tenant_id
```

**Issues**:
- No standard user→tenant mapping logic
- Organization membership changes don't propagate to AudiModal/DeepLake
- Team membership changes are lost in the pipeline
- Multi-organization users create tenant ambiguity

### **5. Billing and Quota Inconsistencies**

**Aether**: Organization-level billing with teams
- Organization pays for all teams and their notebooks
- Team usage rolls up to organization billing

**AudiModal**: Direct tenant billing
- Single tenant responsible for all processing costs
- No organization/team cost allocation

**DeepLake**: Basic tenant quotas
- Simple storage and API quotas per tenant
- No sub-allocation by organization/team

### **6. Search and Discovery Problems**

**Current State**:
- Vector search returns results by tenant_id only
- No organization or team context in search results
- Cannot filter search by "my organization's content" or "my team's content"
- Public/private visibility rules not enforced in vector search

**User Experience Issues**:
- Users see all tenant data in search, regardless of team membership
- Cannot scope search to specific teams or organizations
- Search results don't indicate which organization/team content belongs to

## Proposed Solutions for Organizational Structure Alignment

### **1. Enhanced Tenant-to-Organization Mapping**

**Solution**: Extend AudiModal tenant model to support organizational hierarchy
```go
type Tenant struct {
    ID                  uuid.UUID
    Name                string
    DisplayName         string
    
    // Enhanced organizational context
    AetherOrganizationID *uuid.UUID `json:"aether_organization_id,omitempty"`
    OrganizationName     string      `json:"organization_name,omitempty"`
    
    // Sub-tenant support for teams
    ParentTenantID       *uuid.UUID   `json:"parent_tenant_id,omitempty"`
    TenantType           string       `json:"tenant_type"` // "organization", "team", "user"
    
    // Hierarchical metadata
    OrganizationalPath   string       `json:"organizational_path"` // "/org/team"
    
    // Existing fields...
    BillingPlan         string
    Quotas             TenantQuotas
    Compliance         TenantCompliance
}
```

### **2. Cross-Platform Permission Context**

**Solution**: Embed organizational context in all data models
```go
// AudiModal File enhancement
type File struct {
    // ... existing fields
    
    // Organizational context
    OrganizationID     *uuid.UUID `json:"organization_id,omitempty"`
    TeamID             *uuid.UUID `json:"team_id,omitempty"`
    OwnerID            *uuid.UUID `json:"owner_id,omitempty"`
    
    // Visibility and permissions
    Visibility         string     `json:"visibility"` // private, team, organization, public
    AccessLevel        string     `json:"access_level"` // view, edit, admin
    
    // Permission context for search
    PermissionContext  JSONBMap   `gorm:"type:jsonb" json:"permission_context"`
}
```

```python
# DeepLake vector metadata enhancement
vector_metadata = {
    "tenant_id": str(tenant.id),
    "organization_id": str(organization.id),
    "team_id": str(team.id) if team else None,
    "owner_id": str(user.id),
    "visibility": "team",  # private, team, organization, public
    "permission_context": {
        "allowed_users": ["user1", "user2"],
        "allowed_teams": ["team1"],
        "allowed_organizations": ["org1"]
    }
}
```

### **3. Unified User-Tenant Resolution Service**

**Solution**: Central service for resolving user→tenant mappings
```go
type UserTenantResolver struct {
    aetherClient    *AetherClient
    audiModalClient *AudiModalClient
    deeplakeClient  *DeepLakeClient
}

type UserContext struct {
    UserID          string
    Organizations   []OrganizationContext
    Teams           []TeamContext
    TenantMappings  map[string]string  // org/team ID -> tenant ID
    Permissions     map[string][]string // resource -> permissions
}

func (r *UserTenantResolver) ResolveUserContext(keycloakID string) (*UserContext, error) {
    // 1. Resolve Keycloak ID to Aether internal user ID
    // 2. Get user's organizations and teams from Aether
    // 3. Map organizations/teams to AudiModal tenants
    // 4. Build comprehensive permission context
}
```

### **4. Hierarchical Search and Filtering**

**Solution**: Enhance search with organizational context
```python
class OrganizationalSearchRequest(BaseModel):
    query_vector: List[float]
    
    # Organizational filtering
    organization_ids: Optional[List[str]] = None
    team_ids: Optional[List[str]] = None
    visibility_scope: str = "all"  # "private", "team", "organization", "public", "all"
    
    # Permission-aware search
    user_context: Dict[str, Any]  # Current user's org/team memberships
    enforce_permissions: bool = True
    
    # Standard search options
    options: Optional[SearchOptions] = None

def search_with_organizational_context(request: OrganizationalSearchRequest) -> SearchResponse:
    # Build permission filters based on user context
    permission_filters = build_permission_filters(request.user_context, request.visibility_scope)
    
    # Apply organizational filters to vector search
    enhanced_filters = {
        **permission_filters,
        "organization_id": {"$in": request.organization_ids} if request.organization_ids else {},
        "team_id": {"$in": request.team_ids} if request.team_ids else {},
        "visibility": {"$in": get_allowed_visibility_levels(request.visibility_scope)}
    }
```

### **5. Billing and Quota Hierarchy**

**Solution**: Implement hierarchical billing and quota management
```go
type HierarchicalQuotas struct {
    // Organization-level quotas
    OrganizationQuotas TenantQuotas `json:"organization_quotas"`
    
    // Team-level quotas (subset of org quotas)
    TeamQuotas map[string]TenantQuotas `json:"team_quotas"`
    
    // Current usage tracking
    OrganizationUsage QuotaUsage `json:"organization_usage"`
    TeamUsage        map[string]QuotaUsage `json:"team_usage"`
}

type QuotaEnforcer struct {
    tenantService *TenantService
}

func (q *QuotaEnforcer) CheckQuotaHierarchy(orgID, teamID string, resource string, amount int64) error {
    // 1. Check team quota (if applicable)
    // 2. Check organization quota
    // 3. Ensure team usage + new amount <= team quota
    // 4. Ensure org usage + new amount <= org quota
}
```

### **6. Migration Strategy**

**Phase 1 (Immediate)**:
- Add organizational context fields to AudiModal File model
- Enhance DeepLake vector metadata with org/team context
- Implement basic user→tenant resolution

**Phase 2 (Short-term)**:
- Implement hierarchical tenant support in AudiModal
- Add permission-aware search in DeepLake
- Build organizational filtering in frontend

**Phase 3 (Long-term)**:
- Full hierarchical billing and quota management
- Advanced permission inheritance rules
- Cross-organizational sharing capabilities

## Benefits of Implementation

1. **Preserved User Experience**: Team collaboration and organizational features work end-to-end
2. **Proper Data Isolation**: Respect organizational boundaries in all platforms
3. **Scalable Architecture**: Support enterprise multi-tenant scenarios
4. **Consistent Permissions**: Same access rules across all platforms
5. **Better Search Experience**: Contextual, permission-aware vector search
6. **Proper Billing**: Accurate cost allocation by organization/team

This comprehensive approach addresses the fundamental architectural mismatch while preserving the strengths of each platform's design.

---
*Analysis completed: 2025-01-20*