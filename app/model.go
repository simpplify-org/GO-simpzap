package app

type CreateDeviceRequest struct {
	Number string `json:"number" validate:"required"`
}

type DeleteDeviceRequest struct {
	Number string `json:"number" validate:"required"`
}

type CreateDeviceResponse struct {
	Status   string `json:"status"`
	Endpoint string `json:"endpoint"`
	ID       string `json:"id"`
}

type DeleteDeviceResponse struct {
	Status string `json:"status"`
}
