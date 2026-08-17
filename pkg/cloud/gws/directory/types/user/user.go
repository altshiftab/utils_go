package user

import "github.com/altshiftab/utils_go/pkg/cloud/gws/directory/types/user/name"

type User struct {
	Kind       string `json:"kind,omitempty"`
	Id         string `json:"id,omitempty"`
	Etag       string `json:"etag,omitempty"`
	CustomerId string `json:"customerId,omitempty"`

	PrimaryEmail string     `json:"primaryEmail,omitempty"`
	Name         *name.Name `json:"name,omitempty"`
	Password     string     `json:"password,omitempty"`
	HashFunction string     `json:"hashFunction,omitempty"`

	IsAdmin          bool `json:"isAdmin,omitempty"`
	IsDelegatedAdmin bool `json:"isDelegatedAdmin,omitempty"`
	AgreedToTerms    bool `json:"agreedToTerms,omitempty"`
	Suspended        bool `json:"suspended,omitempty"`
	Archived         bool `json:"archived,omitempty"`
	IsMailboxSetup   bool `json:"isMailboxSetup,omitempty"`

	ChangePasswordAtNextLogin  bool `json:"changePasswordAtNextLogin,omitempty"`
	IpWhitelisted              bool `json:"ipWhitelisted,omitempty"`
	IsEnrolledIn2Sv            bool `json:"isEnrolledIn2Sv,omitempty"`
	IsEnforcedIn2Sv            bool `json:"isEnforcedIn2Sv,omitempty"`
	IncludeInGlobalAddressList bool `json:"includeInGlobalAddressList,omitempty"`

	SuspensionReason string `json:"suspensionReason,omitempty"`
	OrgUnitPath      string `json:"orgUnitPath,omitempty"`
	RecoveryEmail    string `json:"recoveryEmail,omitempty"`
	RecoveryPhone    string `json:"recoveryPhone,omitempty"`
	LastLoginTime    string `json:"lastLoginTime,omitempty"`
	CreationTime     string `json:"creationTime,omitempty"`
}
