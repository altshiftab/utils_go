package mail_folder

// MailFolder is a folder of a mailbox.
type MailFolder struct {
	Id               string `json:"id,omitzero"`
	DisplayName      string `json:"displayName,omitzero"`
	ParentFolderId   string `json:"parentFolderId,omitzero"`
	ChildFolderCount int    `json:"childFolderCount,omitzero"`
	UnreadItemCount  int    `json:"unreadItemCount,omitzero"`
	TotalItemCount   int    `json:"totalItemCount,omitzero"`
}
