package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
)

type apiOperation struct {
	Name        string
	Description string
	Method      string
	Path        string
	Permission  store.Permission
	Handler     gin.HandlerFunc
	Schema      map[string]any
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func (s *Server) apiOperations() []apiOperation {
	id := map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]+$", "description": "Resource ID from the corresponding list operation; numeric IDs must be strings."}
	password := stringSchema("Current password; omit only when the account has no password and uses upstream OAuth.")
	code := stringSchema("Verification code supplied by the account owner.")
	request := stringSchema("Request identifier returned when starting verification.")
	client := func(update bool) map[string]any {
		properties := map[string]any{
			"name": stringSchema("Application name."), "homepage": stringSchema("HTTPS homepage, or HTTP loopback URL."),
			"redirect_uris": map[string]any{"type": "array", "items": stringSchema("Registered callback URL."), "minItems": 1, "maxItems": 20},
			"logo":          stringSchema("Base64 PNG/JPEG without a data URL prefix; up to 512 KiB, 1024 by 1024 pixels."),
		}
		required := []string{"name", "homepage", "redirect_uris"}
		if update {
			properties["id"] = id
			required = append(required, "id")
		} else {
			properties["public"] = map[string]any{"type": "boolean", "description": "Public clients use PKCE without a client secret."}
			required = append(required, "logo")
		}
		return objectSchema(properties, required...)
	}
	key := func(update bool) map[string]any {
		properties := map[string]any{"name": stringSchema("Key name, at most 128 bytes."), "scopes": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": store.KeyPermissions()}, "uniqueItems": true}}
		required := []string{"name", "scopes"}
		if update {
			properties["id"] = id
			required = append(required, "id")
		} else {
			properties["expires_at"] = stringSchema("Optional future expiry in RFC 3339 format.")
		}
		return objectSchema(properties, required...)
	}
	empty := objectSchema(nil)
	byID := objectSchema(map[string]any{"id": id}, "id")
	return []apiOperation{
		{"get_identity", "Read the current account identity.", http.MethodGet, "/me", store.AccountRead, s.apiMe, empty},
		{"get_account_security", "Read account security settings.", http.MethodGet, "/account/security", store.AccountRead, s.securitySettings, empty},
		{"set_password", "Replace your password and revoke other credentials, including this OAuth grant or API key.", http.MethodPut, "/account/password", store.AccountWrite, s.setPassword, objectSchema(map[string]any{"current_password": password, "password": stringSchema("New password, 12–72 bytes.")}, "password")},
		{"start_email_change", "Send a verification code to a new email address.", http.MethodPost, "/account/email", store.AccountWrite, s.startEmailChange, objectSchema(map[string]any{"email": stringSchema("New email address."), "password": password}, "email")},
		{"verify_email_change", "Verify an email change in the same authorization; revokes credentials on success.", http.MethodPost, "/account/email/verify", store.AccountWrite, s.verifyEmailChange, objectSchema(map[string]any{"request": request, "code": code}, "request", "code")},
		{"start_email_binding", "Send a verification code to an additional email address.", http.MethodPost, "/account/emails", store.AccountWrite, s.startEmailBinding, objectSchema(map[string]any{"email": stringSchema("Additional email address."), "password": password}, "email")},
		{"verify_email_binding", "Bind a verified email in the same authorization without changing the primary; revokes credentials.", http.MethodPost, "/account/emails/verify", store.AccountWrite, s.verifyEmailBinding, objectSchema(map[string]any{"request": request, "code": code}, "request", "code")},
		{"set_primary_email", "Select a bound verified email for OAuth claims; revokes credentials.", http.MethodPut, "/account/emails/:id/primary", store.AccountWrite, s.setPrimaryEmail, objectSchema(map[string]any{"id": id, "password": password}, "id")},
		{"remove_email", "Remove a secondary email after password verification; revokes credentials.", http.MethodDelete, "/account/emails/:id", store.AccountWrite, s.removeAccountEmail, objectSchema(map[string]any{"id": id, "password": password}, "id")},
		{"disable_totp", "Remove one authenticator after password and its TOTP verification; revokes credentials.", http.MethodDelete, "/account/totp", store.AccountWrite, s.disableTOTP, objectSchema(map[string]any{"id": map[string]any{"type": "integer", "minimum": 1, "description": "Authenticator device ID; required when multiple devices are bound."}, "password": password, "code": code}, "code")},
		{"list_grants", "List your OAuth authorizations.", http.MethodGet, "/grants", store.GrantsRead, s.apiGrants, empty},
		{"revoke_grant", "Revoke one of your OAuth authorizations and all its tokens.", http.MethodDelete, "/grants/:id", store.GrantsDelete, s.apiRevokeGrant, byID},
		{"list_clients", "List applications visible to your account.", http.MethodGet, "/clients", store.ClientsRead, s.apiClients, empty},
		{"create_client", "Register an application; the client secret is returned once.", http.MethodPost, "/clients", store.ClientsWrite, s.apiCreateClient, client(false)},
		{"update_client", "Edit an application you own, or any application as administrator.", http.MethodPatch, "/clients/:id", store.ClientsWrite, s.apiUpdateClient, client(true)},
		{"delete_client", "Delete an application and invalidate its OAuth tokens.", http.MethodDelete, "/clients/:id", store.ClientsDelete, s.apiDeleteClient, byID},
		{"list_users", "List users; requires administrator role.", http.MethodGet, "/users", store.UsersRead, s.apiListUsers, empty},
		{"update_user_role", "Change another user's role; requires administrator role.", http.MethodPatch, "/users/:id", store.UsersWrite, s.apiSetRole, objectSchema(map[string]any{"id": id, "role": map[string]any{"type": "string", "enum": []string{"user", "admin"}}}, "id", "role")},
		{"ban_user", "Ban another user until a future time and invalidate their credentials; administrators only.", http.MethodPut, "/users/:id/ban", store.UsersWrite, s.apiBanUser, objectSchema(map[string]any{"id": id, "until": stringSchema("Future ban expiry in RFC 3339 format.")}, "id", "until")},
		{"unban_user", "Remove another user's ban; administrators only.", http.MethodDelete, "/users/:id/ban", store.UsersWrite, s.apiUnbanUser, byID},
		{"delete_user", "Delete another user, their applications and credentials; administrators only.", http.MethodDelete, "/users/:id", store.UsersDelete, s.apiDeleteUser, byID},
		{"get_settings", "Read redacted system settings; administrators only.", http.MethodGet, "/settings", store.SettingsRead, s.apiSettings, empty},
		{"update_settings", "Save system settings to configcenter; administrators only. Refresh to activate changes.", http.MethodPut, "/settings", store.SettingsWrite, s.apiSaveSettings, objectSchema(map[string]any{"values": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "minProperties": 1}}, "values")},
		{"refresh_settings", "Reload system settings and report values requiring restart; administrators only.", http.MethodPost, "/settings/refresh", store.SettingsWrite, s.apiRefreshSettings, empty},
		{"get_key_scopes", "List API key permissions you can grant.", http.MethodGet, "/key-scopes", store.KeysRead, s.apiKeyScopes, empty},
		{"list_keys", "List API key metadata you can manage; secrets are never listed.", http.MethodGet, "/keys", store.KeysRead, s.apiListKeys, empty},
		{"create_key", "Create an API key with explicit permissions and optional expiry; token returned once.", http.MethodPost, "/keys", store.KeysWrite, s.apiCreateKey, key(false)},
		{"update_key", "Replace a key's name and permissions; empty scopes disable access.", http.MethodPatch, "/keys/:id", store.KeysWrite, s.apiUpdateKey, key(true)},
		{"delete_key", "Permanently revoke an API key.", http.MethodDelete, "/keys/:id", store.KeysWrite, s.apiDeleteKey, byID},
		{"rotate_key", "Replace an API key token; immediately invalidates the old token and returns the new token once.", http.MethodPost, "/keys/:id/rotate", store.KeysWrite, s.apiRotateKey, byID},
	}
}

func (op apiOperation) authorize(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	if !id.Allows(op.Permission) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_scope"})
		return
	}
	c.Next()
}
