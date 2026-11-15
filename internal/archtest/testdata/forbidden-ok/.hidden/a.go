package a

type (
	Service struct {
		Files       *Files
		Permissions int
	}
	Files  struct{}
	Client struct{}
	File   struct {
		Name    string
		Trashed bool
	}
)

func (*Files) Delete(string) error { return nil }
func (*Client) EmptyTrash()        {}

func f(svc *Service, c *Client) {
	c.EmptyTrash()
	_ = svc.Permissions
	_ = svc.Files.Delete("id")
	_ = File{Trashed: true}
	var f File
	f.Trashed = true
}
