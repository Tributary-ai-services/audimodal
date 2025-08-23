# Simplified Tenant Model: Space-Based Architecture

## Overview

Moving from complex multi-tier hierarchies to a simpler "space" model inspired by Fly.io, where each space maps to a single tenant across all platforms.

## Space Types

### **1. Personal Spaces**
- Each user gets their own personal space
- Maps to a personal tenant in AudiModal/DeepLake
- Private by default, user controls sharing

### **2. Organization Spaces**
- Each Aether organization maps to one tenant
- Shared space for all organization members
- Teams control access to specific notebooks/features within the space

## Architecture Mapping

```
Aether Layer:
User → Personal Space (tenant_id: user-123)
User → Organization Space (tenant_id: org-456)
     └── Teams (control notebook access within org space)

AudiModal Layer:
tenant_id: user-123 (personal space)
tenant_id: org-456 (organization space)

DeepLake Layer:
tenant_id: user-123 (personal datasets)
tenant_id: org-456 (organization datasets)
```

## Implementation Plan

### **Phase 1: Organization → Tenant Mapping**

1. **Extend Organization Model**
```go
type Organization struct {
    // ... existing fields
    TenantID string `json:"tenant_id,omitempty"` // AudiModal/DeepLake tenant ID
    
    // Store tenant credentials
    TenantAPIKey string `json:"-"` // Not serialized, stored securely
}
```

2. **Organization Creation Flow**
```go
func CreateOrganization(req OrganizationCreateRequest, createdBy string) (*Organization, error) {
    // 1. Create organization in Aether
    org := NewOrganization(req, createdBy)
    
    // 2. Create corresponding tenant in AudiModal
    tenant, err := audiModalClient.CreateTenant(CreateTenantRequest{
        Name:        org.Slug,
        DisplayName: org.Name,
        BillingPlan: "organization",
        // ... other fields
    })
    if err != nil {
        return nil, err
    }
    
    // 3. Generate API key for the tenant
    apiKey, err := audiModalClient.CreateAPIKey(tenant.ID)
    if err != nil {
        return nil, err
    }
    
    // 4. Store tenant info in organization
    org.TenantID = tenant.ID.String()
    org.TenantAPIKey = apiKey // Store securely
    
    return org, nil
}
```

### **Phase 2: User Personal Spaces**

1. **Add Personal Tenant to User Model**
```go
type User struct {
    // ... existing fields
    PersonalTenantID string `json:"personal_tenant_id,omitempty"`
    PersonalAPIKey   string `json:"-"` // Stored securely
}
```

2. **User Registration Enhancement**
```go
func CreateUser(req UserCreateRequest) (*User, error) {
    // 1. Create user in Aether
    user := NewUser(req)
    
    // 2. Create personal tenant in AudiModal
    tenant, err := audiModalClient.CreateTenant(CreateTenantRequest{
        Name:        user.Username + "-personal",
        DisplayName: user.Name + "'s Personal Space",
        BillingPlan: "personal",
    })
    
    // 3. Generate personal API key
    apiKey, err := audiModalClient.CreateAPIKey(tenant.ID)
    
    // 4. Store in user record
    user.PersonalTenantID = tenant.ID.String()
    user.PersonalAPIKey = apiKey
    
    return user, nil
}
```

### **Phase 3: Context-Aware Operations**

1. **Space Selection in Frontend**
```javascript
// User selects current working space
const currentSpace = {
    type: 'organization', // or 'personal'
    id: 'org-456',
    name: 'Acme Corp',
    tenantId: 'tenant-789'
};

// All API calls use this context
audiModalService.setCurrentTenant(currentSpace.tenantId);
```

2. **Backend Context Resolution**
```go
type SpaceContext struct {
    SpaceType  string // "personal" or "organization"
    SpaceID    string // user ID or org ID
    TenantID   string // AudiModal/DeepLake tenant ID
    APIKey     string // Tenant-specific API key
    UserRole   string // User's role in this space
}

func ResolveSpaceContext(userID, spaceType, spaceID string) (*SpaceContext, error) {
    switch spaceType {
    case "personal":
        user, err := userService.GetUser(userID)
        if err != nil || user.ID != spaceID {
            return nil, errors.New("unauthorized access to personal space")
        }
        return &SpaceContext{
            SpaceType: "personal",
            SpaceID:   spaceID,
            TenantID:  user.PersonalTenantID,
            APIKey:    user.PersonalAPIKey,
            UserRole:  "owner",
        }, nil
        
    case "organization":
        org, err := orgService.GetOrganization(spaceID)
        if err != nil {
            return nil, err
        }
        
        // Check user membership
        member, err := orgService.GetMember(spaceID, userID)
        if err != nil {
            return nil, errors.New("user not member of organization")
        }
        
        return &SpaceContext{
            SpaceType: "organization",
            SpaceID:   spaceID,
            TenantID:  org.TenantID,
            APIKey:    org.TenantAPIKey,
            UserRole:  member.Role,
        }, nil
    }
}
```

### **Phase 4: Enhanced Data Flow**

1. **Document Upload with Space Context**
```go
func UploadDocument(spaceCtx *SpaceContext, notebookID string, file *File) error {
    // 1. Verify notebook belongs to the space
    notebook, err := notebookService.GetNotebook(notebookID)
    if err != nil {
        return err
    }
    
    // 2. Create AudiModal DataSource for this space
    dataSource, err := audiModalClient.CreateDataSource(spaceCtx.TenantID, CreateDataSourceRequest{
        Name:     notebook.Name,
        Type:     "notebook",
        Metadata: map[string]interface{}{
            "aether_notebook_id": notebookID,
            "aether_space_type": spaceCtx.SpaceType,
            "aether_space_id":   spaceCtx.SpaceID,
        },
    })
    
    // 3. Upload file with space context
    uploadResult, err := audiModalClient.UploadFile(dataSource.ID, file, FileMetadata{
        TenantID:   spaceCtx.TenantID,
        SpaceType:  spaceCtx.SpaceType,
        SpaceID:    spaceCtx.SpaceID,
        NotebookID: notebookID,
    })
    
    return err
}
```

2. **Search with Space Context**
```python
def search_in_space(space_context: Dict, query: str) -> SearchResponse:
    # Filter vectors by tenant_id (space context)
    search_filters = {
        "tenant_id": space_context["tenant_id"],
        # Additional space-specific filters
        "metadata.aether_space_type": space_context["space_type"],
        "metadata.aether_space_id": space_context["space_id"]
    }
    
    return vector_search(query, filters=search_filters)
```

## Teams Within Spaces

Teams remain as access control mechanisms within organization spaces:

1. **Team Access to Notebooks**: Teams control which notebooks members can access within the org space
2. **Team Permissions**: Teams define what members can do (view, edit, admin) within their scope
3. **Space-Level Isolation**: All team activities happen within the organization's tenant space

```go
func CreateNotebook(spaceCtx *SpaceContext, teamID *string, req NotebookCreateRequest) (*Notebook, error) {
    // 1. Verify user has permission in this space
    if spaceCtx.SpaceType == "organization" && teamID != nil {
        // Check team membership and permissions
        canCreate, err := teamService.UserCanCreateNotebooks(spaceCtx.SpaceID, spaceCtx.UserRole, *teamID)
        if err != nil || !canCreate {
            return nil, errors.New("insufficient permissions")
        }
    }
    
    // 2. Create notebook with space context
    notebook := NewNotebook(req, spaceCtx.UserID)
    notebook.TenantID = spaceCtx.TenantID  // All notebooks in this space use same tenant
    notebook.TeamID = teamID               // Optional team assignment
    
    return notebookService.Create(notebook)
}
```

## Migration Strategy

1. **Existing Organizations**: 
   - Create AudiModal tenants for all existing organizations
   - Store tenant IDs and API keys in organization records
   - Migrate existing notebook data to appropriate tenants

2. **Existing Users**: 
   - Create personal tenants for all existing users
   - Store personal tenant info in user records

3. **Existing Data**: 
   - Re-process existing documents with proper tenant context
   - Update vector metadata with space information

## Benefits

1. **Simplicity**: Flat space model instead of complex hierarchies
2. **Clear Isolation**: Each space = one tenant across all platforms  
3. **User Control**: Users choose which space they're working in
4. **Team Flexibility**: Teams control access within organization spaces
5. **Scalability**: Easy to add new space types in the future
6. **Migration Friendly**: Can be implemented incrementally

## Future Enhancements

1. **Additional Space Types**: Project spaces, environment spaces (dev/staging/prod)
2. **RBAC**: Fine-grained role-based access control within spaces
3. **Space Sharing**: Cross-space collaboration features
4. **Space Templates**: Pre-configured space setups for common use cases

---
*Design completed: 2025-01-20*