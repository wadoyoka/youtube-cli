package keychain

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		st   Status
		want Result
	}{
		{"成功は ResultOK", StatusOK, ResultOK},
		{"項目なしは ResultNotFound", StatusItemNotFound, ResultNotFound},
		{"認証失敗は ResultDenied", StatusAuthFailed, ResultDenied},
		{"ユーザーの取り消しは ResultDenied", StatusUserCanceled, ResultDenied},
		{"ダイアログを出せないは ResultNoUI", StatusInteractionNotAllowed, ResultNoUI},
		{"重複は ResultOther", StatusDuplicateItem, ResultOther},
		{"知らない値は ResultOther", Status(-34018), ResultOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			st := tt.st

			// Act
			got := Classify(st)

			// Assert
			if got != tt.want {
				t.Errorf("Classify(%d) = %v, want %v", st, got, tt.want)
			}
		})
	}
}

func TestHardenedRuntime(t *testing.T) {
	tests := []struct {
		name    string
		static  uint32
		dynamic uint32
		want    bool
	}{
		{"codesign -o runtime 付きは true", 0x10002, 0x22011311, true},
		{"go build のままは false", 0x20002, 0x22020201, false},
		{"実行中の状態語に印がなければ false", 0x10002, 0, false},
		{"署名の印がなければ false", 0, 0x10000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			static, dynamic := tt.static, tt.dynamic

			// Act
			got := HardenedRuntime(static, dynamic)

			// Assert
			if got != tt.want {
				t.Errorf("HardenedRuntime(%#x, %#x) = %v, want %v", static, dynamic, got, tt.want)
			}
		})
	}
}
