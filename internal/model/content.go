package model

// FunctionCall represents a function invocation requested by the model.
type FunctionCall struct {
	ID   string         `json:"id,omitempty" bson:"id,omitempty"`
	Name string         `json:"name" bson:"name"`
	Args map[string]any `json:"args,omitempty" bson:"args,omitempty"`
}

// FunctionResponse represents the result of a function invocation.
type FunctionResponse struct {
	ID       string         `json:"id,omitempty" bson:"id,omitempty"`
	Name     string         `json:"name" bson:"name"`
	Response map[string]any `json:"response,omitempty" bson:"response,omitempty"`
}

// Part is a single piece of a conversation turn.
type Part struct {
	Text             string            `json:"text,omitempty" bson:"text,omitempty"`
	FunctionCall     *FunctionCall     `json:"function_call,omitempty" bson:"function_call,omitempty"`
	FunctionResponse *FunctionResponse `json:"function_response,omitempty" bson:"function_response,omitempty"`
}

// Content is a single conversation turn, composed of one or more parts.
type Content struct {
	Parts []Part `json:"parts" bson:"parts"`
	Role  string `json:"role" bson:"role"`
}
