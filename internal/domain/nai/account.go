package nai

type AccountBalance struct {
	PurchasedTrainingSteps int
	FixedTrainingStepsLeft int
	TrialImagesLeft        int
	SubscriptionTier       int
	SubscriptionActive     bool
}
