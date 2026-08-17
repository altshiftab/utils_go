package directory

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/directory_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/list_role_assignments_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/asp"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/group"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/member"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/org_unit"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/privilege"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/role"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/role_assignment"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/token"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/user"
)

const Domain = "admin.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *directory_config.Config
}

func NewClient(options ...directory_config.Option) *Client {
	config := directory_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/admin/directory/v1/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) urlString(path string, query url.Values) string {
	urlObj := *c.baseUrl
	urlObj.Path += path
	if len(query) != 0 {
		urlObj.RawQuery = query.Encode()
	}
	return urlObj.String()
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

// User operations

// CreateUser creates a new user account.
func (c *Client) CreateUser(ctx context.Context, u *user.User, options ...fetch_config.Option) (*user.User, error) {
	if u == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("user"))
	}

	return rest.SendJson[user.User](ctx, http.MethodPost, c.urlString("users", nil), u, c.fetchOptions(options))
}

// GetUser retrieves a user account identified by userKey (primary email address, alias email address, or unique user ID).
func (c *Client) GetUser(ctx context.Context, userKey string, options ...fetch_config.Option) (*user.User, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.GetJson[user.User](ctx, c.urlString("users/"+url.PathEscape(userKey), nil), c.fetchOptions(options))
}

// UpdateUser updates a user account identified by userKey.
func (c *Client) UpdateUser(ctx context.Context, userKey string, u *user.User, options ...fetch_config.Option) (*user.User, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}
	if u == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("user"))
	}

	return rest.SendJson[user.User](
		ctx,
		http.MethodPut,
		c.urlString("users/"+url.PathEscape(userKey), nil),
		u,
		c.fetchOptions(options),
	)
}

// DeleteUser deletes a user account identified by userKey.
func (c *Client) DeleteUser(ctx context.Context, userKey string, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.Do(ctx, http.MethodDelete, c.urlString("users/"+url.PathEscape(userKey), nil), c.fetchOptions(options))
}

type listUsersResponse struct {
	Users         []*user.User `json:"users"`
	NextPageToken string       `json:"nextPageToken"`
}

// ListUsers retrieves all users for the given customer ID (use "my_customer" for the authenticated account).
func (c *Client) ListUsers(ctx context.Context, customer string, options ...fetch_config.Option) ([]*user.User, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{"customer": {customer}}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString("users", query)
		},
		func(response *listUsersResponse) ([]*user.User, string) {
			return response.Users, response.NextPageToken
		},
		c.fetchOptions(options),
	)
}

// Group operations

type listGroupsResponse struct {
	Groups        []*group.Group `json:"groups"`
	NextPageToken string         `json:"nextPageToken"`
}

// ListGroups retrieves all groups for the given customer ID (use "my_customer" for the authenticated account).
func (c *Client) ListGroups(ctx context.Context, customer string, options ...fetch_config.Option) ([]*group.Group, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{"customer": {customer}}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString("groups", query)
		},
		func(response *listGroupsResponse) ([]*group.Group, string) {
			return response.Groups, response.NextPageToken
		},
		c.fetchOptions(options),
	)
}

// CreateGroup creates a new group.
func (c *Client) CreateGroup(ctx context.Context, g *group.Group, options ...fetch_config.Option) (*group.Group, error) {
	if g == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("group"))
	}

	return rest.SendJson[group.Group](ctx, http.MethodPost, c.urlString("groups", nil), g, c.fetchOptions(options))
}

// GetGroup retrieves a group identified by groupKey (group email address, group alias, or unique group ID).
func (c *Client) GetGroup(ctx context.Context, groupKey string, options ...fetch_config.Option) (*group.Group, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}

	return rest.GetJson[group.Group](ctx, c.urlString("groups/"+url.PathEscape(groupKey), nil), c.fetchOptions(options))
}

// UpdateGroup updates a group identified by groupKey.
func (c *Client) UpdateGroup(ctx context.Context, groupKey string, g *group.Group, options ...fetch_config.Option) (*group.Group, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}
	if g == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("group"))
	}

	return rest.SendJson[group.Group](
		ctx,
		http.MethodPut,
		c.urlString("groups/"+url.PathEscape(groupKey), nil),
		g,
		c.fetchOptions(options),
	)
}

// DeleteGroup deletes a group identified by groupKey.
func (c *Client) DeleteGroup(ctx context.Context, groupKey string, options ...fetch_config.Option) error {
	if groupKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}

	return rest.Do(ctx, http.MethodDelete, c.urlString("groups/"+url.PathEscape(groupKey), nil), c.fetchOptions(options))
}

// Group member operations

type listMembersResponse struct {
	Members       []*member.Member `json:"members"`
	NextPageToken string           `json:"nextPageToken"`
}

// ListMembers retrieves all members of a group identified by groupKey.
func (c *Client) ListMembers(ctx context.Context, groupKey string, options ...fetch_config.Option) ([]*member.Member, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString("groups/"+url.PathEscape(groupKey)+"/members", query)
		},
		func(response *listMembersResponse) ([]*member.Member, string) {
			return response.Members, response.NextPageToken
		},
		c.fetchOptions(options),
	)
}

// CreateMember adds a member to a group identified by groupKey.
func (c *Client) CreateMember(ctx context.Context, groupKey string, m *member.Member, options ...fetch_config.Option) (*member.Member, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}
	if m == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("member"))
	}

	return rest.SendJson[member.Member](
		ctx,
		http.MethodPost,
		c.urlString("groups/"+url.PathEscape(groupKey)+"/members", nil),
		m,
		c.fetchOptions(options),
	)
}

// GetMember retrieves a member of a group identified by groupKey and memberKey (member email address or unique member ID).
func (c *Client) GetMember(ctx context.Context, groupKey string, memberKey string, options ...fetch_config.Option) (*member.Member, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}
	if memberKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("member key"))
	}

	return rest.GetJson[member.Member](
		ctx,
		c.urlString("groups/"+url.PathEscape(groupKey)+"/members/"+url.PathEscape(memberKey), nil),
		c.fetchOptions(options),
	)
}

// UpdateMember updates a member of a group identified by groupKey and memberKey.
func (c *Client) UpdateMember(ctx context.Context, groupKey string, memberKey string, m *member.Member, options ...fetch_config.Option) (*member.Member, error) {
	if groupKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}
	if memberKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("member key"))
	}
	if m == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("member"))
	}

	return rest.SendJson[member.Member](
		ctx,
		http.MethodPut,
		c.urlString("groups/"+url.PathEscape(groupKey)+"/members/"+url.PathEscape(memberKey), nil),
		m,
		c.fetchOptions(options),
	)
}

// DeleteMember removes a member from a group identified by groupKey and memberKey.
func (c *Client) DeleteMember(ctx context.Context, groupKey string, memberKey string, options ...fetch_config.Option) error {
	if groupKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("group key"))
	}
	if memberKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("member key"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString("groups/"+url.PathEscape(groupKey)+"/members/"+url.PathEscape(memberKey), nil),
		c.fetchOptions(options),
	)
}

// User security operations

type makeAdminRequest struct {
	Status bool `json:"status"`
}

// MakeUserAdmin grants or revokes super administrator status for the user identified by userKey.
func (c *Client) MakeUserAdmin(ctx context.Context, userKey string, status bool, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.DoWithBody(
		ctx,
		http.MethodPost,
		c.urlString("users/"+url.PathEscape(userKey)+"/makeAdmin", nil),
		&makeAdminRequest{Status: status},
		c.fetchOptions(options),
	)
}

// SignOutUser signs the user identified by userKey out of all web and device sessions and resets their sign-in cookies.
func (c *Client) SignOutUser(ctx context.Context, userKey string, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.Do(ctx, http.MethodPost, c.urlString("users/"+url.PathEscape(userKey)+"/signOut", nil), c.fetchOptions(options))
}

// TurnOffUser2Sv turns off 2-step verification for the user identified by userKey.
func (c *Client) TurnOffUser2Sv(ctx context.Context, userKey string, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.Do(
		ctx,
		http.MethodPost,
		c.urlString("users/"+url.PathEscape(userKey)+"/twoStepVerification/turnOff", nil),
		c.fetchOptions(options),
	)
}

// Org unit operations

// orgUnitPathSegment builds the URL path for an org unit. Slashes in the org
// unit path are segment separators and must not be escaped.
func orgUnitPathSegment(customer string, orgUnitPath string) string {
	return "customer/" + url.PathEscape(customer) + "/orgunits/" + strings.TrimPrefix(orgUnitPath, "/")
}

// CreateOrgUnit creates a new organizational unit for the given customer ID (use "my_customer" for the authenticated account).
func (c *Client) CreateOrgUnit(ctx context.Context, customer string, ou *org_unit.OrgUnit, options ...fetch_config.Option) (*org_unit.OrgUnit, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if ou == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("org unit"))
	}

	return rest.SendJson[org_unit.OrgUnit](
		ctx,
		http.MethodPost,
		c.urlString("customer/"+url.PathEscape(customer)+"/orgunits", nil),
		ou,
		c.fetchOptions(options),
	)
}

// GetOrgUnit retrieves an organizational unit identified by orgUnitPath (e.g. "/Engineering/Frontend") or unique ID (e.g. "id:03ph8a2z1xdnme9").
func (c *Client) GetOrgUnit(ctx context.Context, customer string, orgUnitPath string, options ...fetch_config.Option) (*org_unit.OrgUnit, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if orgUnitPath == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("org unit path"))
	}

	return rest.GetJson[org_unit.OrgUnit](
		ctx,
		c.urlString(orgUnitPathSegment(customer, orgUnitPath), nil),
		c.fetchOptions(options),
	)
}

// UpdateOrgUnit updates an organizational unit identified by orgUnitPath or unique ID.
func (c *Client) UpdateOrgUnit(ctx context.Context, customer string, orgUnitPath string, ou *org_unit.OrgUnit, options ...fetch_config.Option) (*org_unit.OrgUnit, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if orgUnitPath == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("org unit path"))
	}
	if ou == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("org unit"))
	}

	return rest.SendJson[org_unit.OrgUnit](
		ctx,
		http.MethodPut,
		c.urlString(orgUnitPathSegment(customer, orgUnitPath), nil),
		ou,
		c.fetchOptions(options),
	)
}

// DeleteOrgUnit deletes an organizational unit identified by orgUnitPath or unique ID.
func (c *Client) DeleteOrgUnit(ctx context.Context, customer string, orgUnitPath string, options ...fetch_config.Option) error {
	if customer == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if orgUnitPath == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("org unit path"))
	}

	return rest.Do(ctx, http.MethodDelete, c.urlString(orgUnitPathSegment(customer, orgUnitPath), nil), c.fetchOptions(options))
}

type listOrgUnitsResponse struct {
	OrganizationUnits []*org_unit.OrgUnit `json:"organizationUnits"`
}

// ListOrgUnits retrieves all organizational units for the given customer ID (use "my_customer" for the authenticated account).
func (c *Client) ListOrgUnits(ctx context.Context, customer string, options ...fetch_config.Option) ([]*org_unit.OrgUnit, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	response, err := rest.GetJson[listOrgUnitsResponse](
		ctx,
		c.urlString("customer/"+url.PathEscape(customer)+"/orgunits", url.Values{"type": {"all"}}),
		c.fetchOptions(options),
	)
	if err != nil {
		return nil, err
	}

	return response.OrganizationUnits, nil
}

// Role operations

type listRolesResponse struct {
	Items         []*role.Role `json:"items"`
	NextPageToken string       `json:"nextPageToken"`
}

// ListRoles retrieves all roles for the given customer ID (use "my_customer" for the authenticated account).
func (c *Client) ListRoles(ctx context.Context, customer string, options ...fetch_config.Option) ([]*role.Role, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString("customer/"+url.PathEscape(customer)+"/roles", query)
		},
		func(response *listRolesResponse) ([]*role.Role, string) {
			return response.Items, response.NextPageToken
		},
		c.fetchOptions(options),
	)
}

// CreateRole creates a new role for the given customer ID.
func (c *Client) CreateRole(ctx context.Context, customer string, r *role.Role, options ...fetch_config.Option) (*role.Role, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if r == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("role"))
	}

	return rest.SendJson[role.Role](
		ctx,
		http.MethodPost,
		c.urlString("customer/"+url.PathEscape(customer)+"/roles", nil),
		r,
		c.fetchOptions(options),
	)
}

// GetRole retrieves a role identified by roleId.
func (c *Client) GetRole(ctx context.Context, customer string, roleId string, options ...fetch_config.Option) (*role.Role, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if roleId == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("role id"))
	}

	return rest.GetJson[role.Role](
		ctx,
		c.urlString("customer/"+url.PathEscape(customer)+"/roles/"+url.PathEscape(roleId), nil),
		c.fetchOptions(options),
	)
}

// UpdateRole updates a role identified by roleId.
func (c *Client) UpdateRole(ctx context.Context, customer string, roleId string, r *role.Role, options ...fetch_config.Option) (*role.Role, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if roleId == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("role id"))
	}
	if r == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("role"))
	}

	return rest.SendJson[role.Role](
		ctx,
		http.MethodPut,
		c.urlString("customer/"+url.PathEscape(customer)+"/roles/"+url.PathEscape(roleId), nil),
		r,
		c.fetchOptions(options),
	)
}

// DeleteRole deletes a role identified by roleId.
func (c *Client) DeleteRole(ctx context.Context, customer string, roleId string, options ...fetch_config.Option) error {
	if customer == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if roleId == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("role id"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString("customer/"+url.PathEscape(customer)+"/roles/"+url.PathEscape(roleId), nil),
		c.fetchOptions(options),
	)
}

type listPrivilegesResponse struct {
	Items []*privilege.Privilege `json:"items"`
}

// ListPrivileges retrieves the privileges supported for building custom roles for the given customer ID.
func (c *Client) ListPrivileges(ctx context.Context, customer string, options ...fetch_config.Option) ([]*privilege.Privilege, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	response, err := rest.GetJson[listPrivilegesResponse](
		ctx,
		c.urlString("customer/"+url.PathEscape(customer)+"/roles/ALL/privileges", nil),
		c.fetchOptions(options),
	)
	if err != nil {
		return nil, err
	}

	return response.Items, nil
}

// Role assignment operations

type listRoleAssignmentsResponse struct {
	Items         []*role_assignment.RoleAssignment `json:"items"`
	NextPageToken string                            `json:"nextPageToken"`
}

// ListRoleAssignments retrieves all role assignments for the given customer ID, optionally filtered by user key or role ID.
func (c *Client) ListRoleAssignments(ctx context.Context, customer string, options ...list_role_assignments_config.Option) ([]*role_assignment.RoleAssignment, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}

	listRoleAssignmentsConfig := list_role_assignments_config.New(options...)

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{}
			if listRoleAssignmentsConfig.UserKey != "" {
				query.Set("userKey", listRoleAssignmentsConfig.UserKey)
			}
			if listRoleAssignmentsConfig.RoleId != "" {
				query.Set("roleId", listRoleAssignmentsConfig.RoleId)
			}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString("customer/"+url.PathEscape(customer)+"/roleassignments", query)
		},
		func(response *listRoleAssignmentsResponse) ([]*role_assignment.RoleAssignment, string) {
			return response.Items, response.NextPageToken
		},
		c.fetchOptions(listRoleAssignmentsConfig.FetchOptions),
	)
}

// CreateRoleAssignment creates a new role assignment for the given customer ID.
func (c *Client) CreateRoleAssignment(ctx context.Context, customer string, ra *role_assignment.RoleAssignment, options ...fetch_config.Option) (*role_assignment.RoleAssignment, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if ra == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("role assignment"))
	}

	return rest.SendJson[role_assignment.RoleAssignment](
		ctx,
		http.MethodPost,
		c.urlString("customer/"+url.PathEscape(customer)+"/roleassignments", nil),
		ra,
		c.fetchOptions(options),
	)
}

// GetRoleAssignment retrieves a role assignment identified by roleAssignmentId.
func (c *Client) GetRoleAssignment(ctx context.Context, customer string, roleAssignmentId string, options ...fetch_config.Option) (*role_assignment.RoleAssignment, error) {
	if customer == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if roleAssignmentId == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("role assignment id"))
	}

	return rest.GetJson[role_assignment.RoleAssignment](
		ctx,
		c.urlString("customer/"+url.PathEscape(customer)+"/roleassignments/"+url.PathEscape(roleAssignmentId), nil),
		c.fetchOptions(options),
	)
}

// DeleteRoleAssignment deletes a role assignment identified by roleAssignmentId.
func (c *Client) DeleteRoleAssignment(ctx context.Context, customer string, roleAssignmentId string, options ...fetch_config.Option) error {
	if customer == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("customer"))
	}
	if roleAssignmentId == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("role assignment id"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString("customer/"+url.PathEscape(customer)+"/roleassignments/"+url.PathEscape(roleAssignmentId), nil),
		c.fetchOptions(options),
	)
}

// Token operations

type listTokensResponse struct {
	Items []*token.Token `json:"items"`
}

// ListTokens retrieves the OAuth tokens issued to third-party applications for the user identified by userKey.
func (c *Client) ListTokens(ctx context.Context, userKey string, options ...fetch_config.Option) ([]*token.Token, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	response, err := rest.GetJson[listTokensResponse](
		ctx,
		c.urlString("users/"+url.PathEscape(userKey)+"/tokens", nil),
		c.fetchOptions(options),
	)
	if err != nil {
		return nil, err
	}

	return response.Items, nil
}

// GetToken retrieves the OAuth token issued to the third-party application identified by clientId for the user identified by userKey.
func (c *Client) GetToken(ctx context.Context, userKey string, clientId string, options ...fetch_config.Option) (*token.Token, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}
	if clientId == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("client id"))
	}

	return rest.GetJson[token.Token](
		ctx,
		c.urlString("users/"+url.PathEscape(userKey)+"/tokens/"+url.PathEscape(clientId), nil),
		c.fetchOptions(options),
	)
}

// DeleteToken revokes the OAuth token issued to the third-party application identified by clientId for the user identified by userKey.
func (c *Client) DeleteToken(ctx context.Context, userKey string, clientId string, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}
	if clientId == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("client id"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString("users/"+url.PathEscape(userKey)+"/tokens/"+url.PathEscape(clientId), nil),
		c.fetchOptions(options),
	)
}

// Application-specific password operations

type listAspsResponse struct {
	Items []*asp.Asp `json:"items"`
}

// ListAsps retrieves the application-specific passwords issued for the user identified by userKey.
func (c *Client) ListAsps(ctx context.Context, userKey string, options ...fetch_config.Option) ([]*asp.Asp, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	response, err := rest.GetJson[listAspsResponse](
		ctx,
		c.urlString("users/"+url.PathEscape(userKey)+"/asps", nil),
		c.fetchOptions(options),
	)
	if err != nil {
		return nil, err
	}

	return response.Items, nil
}

// GetAsp retrieves the application-specific password identified by codeId for the user identified by userKey.
func (c *Client) GetAsp(ctx context.Context, userKey string, codeId int, options ...fetch_config.Option) (*asp.Asp, error) {
	if userKey == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.GetJson[asp.Asp](
		ctx,
		c.urlString("users/"+url.PathEscape(userKey)+"/asps/"+strconv.Itoa(codeId), nil),
		c.fetchOptions(options),
	)
}

// DeleteAsp revokes the application-specific password identified by codeId for the user identified by userKey.
func (c *Client) DeleteAsp(ctx context.Context, userKey string, codeId int, options ...fetch_config.Option) error {
	if userKey == "" {
		return motmedelErrors.NewWithTrace(empty_error.New("user key"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString("users/"+url.PathEscape(userKey)+"/asps/"+strconv.Itoa(codeId), nil),
		c.fetchOptions(options),
	)
}
