package container

import (
	app "vision-bot/internal/application"
	"vision-bot/internal/domain/port"
)

type Container struct {
	UserService       *app.UserService
	InspectionService *app.InspectionService
}

func New(userRepo port.UserRepository, detector port.DefectDetector, describer port.DefectDescriber) *Container {
	userService := app.NewUserService(userRepo)
	inspectionService := app.NewInspectionService(userService, detector, describer)

	return &Container{
		UserService:       userService,
		InspectionService: inspectionService,
	}
}
