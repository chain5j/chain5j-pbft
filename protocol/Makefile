mock-gen:
	mockgen -destination ./mockpbft/mock_protocol.go -package mockpbft -source protocol.go
	mockgen -destination ./mockpbft/mock_validator.go -package mockpbft -source validator.go

mock-dep:
	go get -u github.com/golang/mock/gomock
	go get -u github.com/golang/mock/mockgen
