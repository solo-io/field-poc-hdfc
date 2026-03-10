package auth

import "errors"

var (
	ErrForbidden    = errors.New("forbidden")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidToken = errors.New("invalid token")
)

// CanViewBudget checks if the identity can view a budget.
// Org users can view all budgets within their org (including team budgets).
// Team users can only view their team's budgets.
func CanViewBudget(identity *Identity, ownerOrgID, ownerTeamID string) bool {
	if identity == nil {
		return false
	}

	// Org admin can view all org and team budgets
	if identity.IsOrg && identity.OrgID != "" && identity.OrgID == ownerOrgID {
		return true
	}

	// Team member can view their team's budgets
	if identity.TeamID != "" && identity.TeamID == ownerTeamID {
		return true
	}

	// If the budget has no owner, allow viewing (backwards compatibility)
	if ownerOrgID == "" && ownerTeamID == "" {
		return true
	}

	return false
}

// CanManageBudget checks if the identity can create, update, or delete a budget.
// Uses the same rules as CanViewBudget.
func CanManageBudget(identity *Identity, ownerOrgID, ownerTeamID string) bool {
	return CanViewBudget(identity, ownerOrgID, ownerTeamID)
}

// CanCreateBudgetForOrg checks if the identity can create a budget for an org.
// Only org members can create org-level budgets.
func CanCreateBudgetForOrg(identity *Identity, orgID string) bool {
	if identity == nil {
		return false
	}

	return identity.IsOrg && identity.OrgID == orgID
}

// CanCreateBudgetForTeam checks if the identity can create a budget for a team.
// Org admins can create budgets for any team in their org.
// Team members can create budgets for any team within their org.
func CanCreateBudgetForTeam(identity *Identity, orgID, teamID string) bool {
	if identity == nil {
		return false
	}

	// Org admin can create budgets for any team in their org
	if identity.IsOrg && identity.OrgID == orgID {
		return true
	}

	// Team member can create budgets for any team within their org
	// (They have org_id set to their parent org)
	if !identity.IsOrg && identity.OrgID != "" && identity.OrgID == orgID {
		return true
	}

	return false
}

// FilterBudgetsByIdentity filters a list of budget ownership info based on identity permissions.
// Returns indices of budgets the identity can view.
type BudgetOwnership struct {
	Index       int
	OwnerOrgID  string
	OwnerTeamID string
}

func FilterBudgetsByIdentity(identity *Identity, budgets []BudgetOwnership) []int {
	var allowed []int

	for _, b := range budgets {
		if CanViewBudget(identity, b.OwnerOrgID, b.OwnerTeamID) {
			allowed = append(allowed, b.Index)
		}
	}

	return allowed
}

// GetOwnershipFromIdentity returns the ownership values to set when creating a budget.
// Both org admins and team members have org_id set.
// Team members additionally have team_id set.
func GetOwnershipFromIdentity(identity *Identity) (orgID, teamID string) {
	if identity == nil {
		return "", ""
	}

	// Both org admins and team members have org_id
	// Team members additionally have team_id
	return identity.OrgID, identity.TeamID
}
