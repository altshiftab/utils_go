package group

import (
	"encoding/json/jsontext"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/force_send"
)

type Group struct {
	Kind        string `json:"kind,omitempty"`
	Email       string `json:"email,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	WhoCanJoin           string `json:"whoCanJoin,omitempty"`
	WhoCanViewMembership string `json:"whoCanViewMembership,omitempty"`
	WhoCanViewGroup      string `json:"whoCanViewGroup,omitempty"`
	WhoCanPostMessage    string `json:"whoCanPostMessage,omitempty"`
	WhoCanLeaveGroup     string `json:"whoCanLeaveGroup,omitempty"`
	WhoCanContactOwner   string `json:"whoCanContactOwner,omitempty"`
	WhoCanDiscoverGroup  string `json:"whoCanDiscoverGroup,omitempty"`

	AllowExternalMembers string `json:"allowExternalMembers,omitempty"`
	AllowWebPosting      string `json:"allowWebPosting,omitempty"`
	PrimaryLanguage      string `json:"primaryLanguage,omitempty"`

	IsArchived  string `json:"isArchived,omitempty"`
	ArchiveOnly string `json:"archiveOnly,omitempty"`

	MessageModerationLevel string `json:"messageModerationLevel,omitempty"`
	SpamModerationLevel    string `json:"spamModerationLevel,omitempty"`

	ReplyTo                            string `json:"replyTo,omitempty"`
	CustomReplyTo                      string `json:"customReplyTo,omitempty"`
	IncludeCustomFooter                string `json:"includeCustomFooter,omitempty"`
	CustomFooterText                   string `json:"customFooterText,omitempty"`
	SendMessageDenyNotification        string `json:"sendMessageDenyNotification,omitempty"`
	DefaultMessageDenyNotificationText string `json:"defaultMessageDenyNotificationText,omitempty"`

	MembersCanPostAsTheGroup   string `json:"membersCanPostAsTheGroup,omitempty"`
	IncludeInGlobalAddressList string `json:"includeInGlobalAddressList,omitempty"`
	FavoriteRepliesOnTop       string `json:"favoriteRepliesOnTop,omitempty"`

	WhoCanModerateMembers string `json:"whoCanModerateMembers,omitempty"`
	WhoCanModerateContent string `json:"whoCanModerateContent,omitempty"`
	WhoCanAssistContent   string `json:"whoCanAssistContent,omitempty"`

	// CustomRolesEnabledForSettingsToBeMerged is read-only; UPDATE and PATCH requests to it are ignored.
	CustomRolesEnabledForSettingsToBeMerged string `json:"customRolesEnabledForSettingsToBeMerged,omitempty"`

	EnableCollaborativeInbox string `json:"enableCollaborativeInbox,omitempty"`
	DefaultSender            string `json:"defaultSender,omitempty"`

	//// Deprecated fields below.
	//
	//// Deprecated: merged into WhoCanModerateMembers.
	//WhoCanInvite string `json:"whoCanInvite,omitempty"`
	//// Deprecated: merged into WhoCanModerateMembers.
	//WhoCanAdd string `json:"whoCanAdd,omitempty"`
	//// Deprecated: merged into WhoCanModerateMembers.
	//WhoCanApproveMembers string `json:"whoCanApproveMembers,omitempty"`
	//// Deprecated: merged into WhoCanModerateMembers.
	//WhoCanBanUsers string `json:"whoCanBanUsers,omitempty"`
	//// Deprecated: merged into WhoCanModerateMembers.
	//WhoCanModifyMembers string `json:"whoCanModifyMembers,omitempty"`
	//
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanApproveMessages string `json:"whoCanApproveMessages,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanDeleteAnyPost string `json:"whoCanDeleteAnyPost,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanDeleteTopics string `json:"whoCanDeleteTopics,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanLockTopics string `json:"whoCanLockTopics,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanMoveTopicsIn string `json:"whoCanMoveTopicsIn,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanMoveTopicsOut string `json:"whoCanMoveTopicsOut,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanPostAnnouncements string `json:"whoCanPostAnnouncements,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanHideAbuse string `json:"whoCanHideAbuse,omitempty"`
	//// Deprecated: merged into WhoCanModerateContent.
	//WhoCanMakeTopicsSticky string `json:"whoCanMakeTopicsSticky,omitempty"`
	//
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanAssignTopics string `json:"whoCanAssignTopics,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanUnassignTopic string `json:"whoCanUnassignTopic,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanTakeTopics string `json:"whoCanTakeTopics,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanMarkDuplicate string `json:"whoCanMarkDuplicate,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanMarkNoResponseNeeded string `json:"whoCanMarkNoResponseNeeded,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanMarkFavoriteReplyOnAnyTopic string `json:"whoCanMarkFavoriteReplyOnAnyTopic,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanMarkFavoriteReplyOnOwnTopic string `json:"whoCanMarkFavoriteReplyOnOwnTopic,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanUnmarkFavoriteReplyOnAnyTopic string `json:"whoCanUnmarkFavoriteReplyOnAnyTopic,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanEnterFreeFormTags string `json:"whoCanEnterFreeFormTags,omitempty"`
	//// Deprecated: merged into WhoCanAssistContent.
	//WhoCanModifyTagsAndCategories string `json:"whoCanModifyTagsAndCategories,omitempty"`
	//
	//// Deprecated: merged into WhoCanDiscoverGroup.
	//ShowInGroupDirectory string `json:"showInGroupDirectory,omitempty"`
	//
	//// Deprecated: always "NONE".
	//WhoCanAddReferences string `json:"whoCanAddReferences,omitempty"`
	//
	//// Deprecated: maximum size of a message is 25 MB.
	//MaxMessageBytes int `json:"maxMessageBytes,omitempty"`
	//
	//// Deprecated: always DEFAULT_FONT.
	//MessageDisplayFont string `json:"messageDisplayFont,omitempty"`
	//
	//// Deprecated.
	//AllowGoogleCommunication string `json:"allowGoogleCommunication,omitempty"`

	// ForceSendFields names, by their JSON names, the fields a request sends even where they are
	// empty: left out, the API keeps the value it had, so sending one empty is how it is cleared.
	ForceSendFields []string `json:"-"`
}

// MarshalJSONTo writes the group with the fields ForceSendFields names even where they are empty.
func (g *Group) MarshalJSONTo(encoder *jsontext.Encoder) error {
	type plain Group
	encoded, err := force_send.Marshal((*plain)(g), g.ForceSendFields)
	if err != nil {
		return err
	}

	return encoder.WriteValue(encoded)
}
