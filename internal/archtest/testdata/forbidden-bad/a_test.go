package a

func g(svc *Service, c *Client) {
	c.EmptyTrash()
	_ = svc.Permissions
	_ = svc.Files.Delete("id")
	_ = File{Trashed: true}
	var f File
	f.Trashed = true
}
