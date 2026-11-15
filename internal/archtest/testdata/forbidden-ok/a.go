package a

type (
	Service struct{ Files *Files }
	Files   struct{}
	File    struct {
		Name    string
		InTrash bool
	}
)

func (*Files) List() error      { return nil }
func (*Files) Get(string) error { return nil }

func f(svc *Service) {
	_ = svc.Files.List()
	_ = svc.Files.Get("id")
	_ = File{Name: "x", InTrash: true}
}
