# Space-Based Implementation Plan

## Overview

Implementation plan for the simplified space-based tenant model, moving from complex hierarchies to a Fly.io-inspired space architecture.

## Architecture Summary

- **Personal Spaces**: Each user gets a personal tenant
- **Organization Spaces**: Each organization maps to one tenant
- **Teams**: Control access to notebooks/features within organization spaces
- **Simple Mapping**: One space = one tenant across all platforms

## Implementation Todo List

### **Phase 1: Backend Foundation**

#### **1.1 Extend Organization Model**
- [ ] Add `TenantID` field to Organization struct
- [ ] Add `TenantAPIKey` field (stored securely, not serialized)
- [ ] Update database migration for organizations table
- [ ] Add validation for tenant fields
- [ ] Update organization creation flow to create AudiModal tenant
- [ ] Update organization deletion flow to clean up tenant

#### **1.2 Extend User Model** 
- [ ] Add `PersonalTenantID` field to User struct
- [ ] Add `PersonalAPIKey` field (stored securely)
- [ ] Update database migration for users table
- [ ] Update user registration to create personal tenant
- [ ] Update user deletion to clean up personal tenant

#### **1.3 Create Space Context System**
- [ ] Create `SpaceContext` struct with space type, ID, tenant info
- [ ] Implement `ResolveSpaceContext()` function
- [ ] Add space context validation and authorization
- [ ] Create middleware for space context resolution
- [ ] Add space context to request handling

#### **1.4 AudiModal Integration**
- [ ] Create AudiModal client for tenant management
- [ ] Implement `CreateTenant()` API call
- [ ] Implement `DeleteTenant()` API call  
- [ ] Implement `CreateAPIKey()` for tenants
- [ ] Add error handling for tenant operations
- [ ] Add retry logic for tenant API calls

### **Phase 2: Data Flow Integration**

#### **2.1 Update Notebook Operations**
- [ ] Modify notebook creation to use space context
- [ ] Update notebook listing to filter by space
- [ ] Add space validation to notebook operations
- [ ] Update notebook sharing within space context
- [ ] Ensure team permissions work within organization spaces

#### **2.2 Update Document Operations**
- [ ] Modify document upload to use space context
- [ ] Update AudiModal DataSource creation with space metadata
- [ ] Pass space context to file processing pipeline
- [ ] Update document listing to filter by space
- [ ] Add space information to document metadata

#### **2.3 Update Search Operations** 
- [ ] Add space filtering to vector search
- [ ] Update DeepLake vector metadata with space info
- [ ] Implement space-aware search results
- [ ] Add space context to search API endpoints
- [ ] Test cross-space data isolation

### **Phase 3: Frontend Implementation**

#### **3.1 Space Selection UI**
- [ ] Create space selector component
- [ ] Add space switching functionality
- [ ] Store current space in Redux state
- [ ] Add space indicator in navigation
- [ ] Handle space switching across all views

#### **3.2 Update API Integration**
- [ ] Modify all API calls to include space context
- [ ] Update audiModalService to use current space tenant
- [ ] Add space validation to API requests
- [ ] Handle space-related error states
- [ ] Add loading states for space operations

#### **3.3 UI Updates**
- [ ] Update notebook views to show space context
- [ ] Update document views with space information
- [ ] Add space-specific settings pages
- [ ] Update organization management for tenant info
- [ ] Add personal space management UI

### **Phase 4: Migration and Data Cleanup**

#### **4.1 Existing Organization Migration**
- [ ] Create migration script for existing organizations
- [ ] Generate AudiModal tenants for all orgs
- [ ] Store tenant IDs and API keys in org records
- [ ] Validate all organizations have valid tenants
- [ ] Handle migration errors and rollback plan

#### **4.2 Existing User Migration**
- [ ] Create migration script for existing users
- [ ] Generate personal tenants for all users
- [ ] Store personal tenant info in user records
- [ ] Validate all users have personal tenants
- [ ] Handle migration errors and rollback plan

#### **4.3 Data Re-processing**
- [ ] Identify existing documents needing re-processing
- [ ] Update document metadata with space context
- [ ] Re-process documents through AudiModal with correct tenant
- [ ] Update vector metadata in DeepLake with space info
- [ ] Verify data integrity after migration

### **Phase 5: Testing and Validation**

#### **5.1 Unit Tests**
- [ ] Test space context resolution
- [ ] Test tenant creation/deletion flows
- [ ] Test space validation and authorization
- [ ] Test API integration with space context
- [ ] Test error handling for tenant operations

#### **5.2 Integration Tests**
- [ ] Test end-to-end document upload with spaces
- [ ] Test search results are space-isolated
- [ ] Test user switching between spaces
- [ ] Test organization member access within spaces
- [ ] Test team permissions within organization spaces

#### **5.3 Migration Testing**
- [ ] Test organization migration script
- [ ] Test user migration script
- [ ] Test data re-processing scripts
- [ ] Validate no data leakage between spaces
- [ ] Test rollback procedures

### **Phase 6: Deployment and Monitoring**

#### **6.1 Deployment Preparation**
- [ ] Create deployment scripts for database migrations
- [ ] Prepare rollback plans for each migration step
- [ ] Set up monitoring for tenant operations
- [ ] Configure alerts for space-related errors
- [ ] Prepare documentation for space operations

#### **6.2 Staged Rollout**
- [ ] Deploy backend changes with feature flags
- [ ] Test space operations in staging environment
- [ ] Migrate small batch of test organizations
- [ ] Deploy frontend changes with space selection
- [ ] Monitor for errors and performance issues

#### **6.3 Full Migration**
- [ ] Migrate all existing organizations
- [ ] Migrate all existing users
- [ ] Re-process all existing data
- [ ] Enable space-based operations for all users
- [ ] Remove old non-space code paths

### **Phase 7: Future Enhancements**

#### **7.1 Additional Space Types**
- [ ] Design project space type
- [ ] Design environment space type (dev/staging/prod)
- [ ] Implement space type creation UI
- [ ] Add space type-specific features
- [ ] Add space templates for common setups

#### **7.2 Enhanced RBAC**
- [ ] Design fine-grained permissions within spaces
- [ ] Implement role-based access control
- [ ] Add permission management UI
- [ ] Add audit logging for space operations
- [ ] Add compliance features for spaces

#### **7.3 Cross-Space Features**
- [ ] Design cross-space sharing mechanisms
- [ ] Implement space-to-space collaboration
- [ ] Add space discovery and joining features
- [ ] Add space analytics and usage tracking
- [ ] Add space backup and restore features

## Success Criteria

### **Phase 1-3 Complete:**
- [ ] Organizations automatically create AudiModal tenants
- [ ] Users have personal tenants for private work
- [ ] All operations work within space context
- [ ] Frontend shows current space and allows switching
- [ ] No data leakage between spaces

### **Phase 4-6 Complete:**
- [ ] All existing data migrated to space model
- [ ] No breaking changes for existing users
- [ ] Search results properly isolated by space
- [ ] Teams work correctly within organization spaces
- [ ] System performs well with space overhead

### **Phase 7 Complete:**
- [ ] Additional space types available
- [ ] Enhanced permissions and collaboration
- [ ] Rich space management features
- [ ] Analytics and monitoring for spaces
- [ ] Full feature parity with previous complex model

## Risk Mitigation

### **High-Risk Items:**
1. **Data Migration**: Extensive testing required, rollback plan essential
2. **Tenant Creation**: Handle AudiModal API failures gracefully
3. **Cross-Platform Consistency**: Ensure tenant IDs work across all platforms
4. **Performance**: Monitor overhead of space context resolution

### **Mitigation Strategies:**
- Feature flags for gradual rollout
- Comprehensive testing at each phase
- Monitoring and alerting for all space operations
- Clear rollback procedures for each migration step
- Regular backups before major changes

---
*Implementation plan created: 2025-01-20*