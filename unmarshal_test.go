package ghb

import (
	"testing"
	"time"

	"github.com/malayanand/ghb/test"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_extractURLParams(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:    "Single parameter",
			pattern: "v1/getUsers/{id}",
			path:    "v1/getUsers/123",
			expected: map[string]string{
				"id": "123",
			},
		},
		{
			name:    "Multiple parameters",
			pattern: "v1/getUsers/{id}/workspace/{wid}",
			path:    "v1/getUsers/123/workspace/456",
			expected: map[string]string{
				"id":  "123",
				"wid": "456",
			},
		},
		{
			name:     "No parameters",
			pattern:  "v1/getUsers",
			path:     "v1/getUsers",
			expected: map[string]string{},
		},
		{
			name:    "Special characters parameter",
			pattern: "v1/getUsers/{user-id}/posts/{post-id}",
			path:    "v1/getUsers/user-123/posts/1",
			expected: map[string]string{
				"user-id": "user-123",
				"post-id": "1",
			},
		},
		{
			name:    "Trailing slashes",
			pattern: "v1/getUsers/{id}",
			path:    "v1/getUsers/123/",
			expected: map[string]string{
				"id": "123",
			},
		},
		{
			name:    "Missing patterns",
			pattern: "v1/getUsers/{id}/workspace/{wid}",
			path:    "v1/getUsers/123/",
			expected: map[string]string{
				"id": "123",
			},
		},
		{
			name:     "Missing parts",
			pattern:  "v1/getUsers/{id}/posts/{pid}",
			path:     "v1/getUsers/123//posts/22",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, _ := extractURLParams(tt.pattern, tt.path)
			require.Equal(t, actual, tt.expected)
		})
	}
}

func Test_unmarshalBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    []byte
		params   map[string]string
		options  *unmarshalOptions
		expected *test.TestUser
		isErr    bool
	}{
		{
			name:  "when bytes are provided",
			bytes: []byte(`{"id": "123"}`),
			expected: &test.TestUser{
				Id: "123",
			},
		},
		{
			name:  "when params are provided",
			bytes: nil,
			params: map[string]string{
				"id": "123",
			},
			expected: &test.TestUser{
				Id: "123",
			},
		},
		{
			name:  "when bytes and params are provided",
			bytes: []byte(`{"id": "123", "name": "John Doe", "age": 30, "created_at": "2025-09-21T11:32:03.000Z"}`),
			params: map[string]string{
				"id": "123456",
			},
			options: &unmarshalOptions{
				timeFormat: ISOTimeFormat,
			},
			expected: &test.TestUser{
				Id:        "123", // should be overridden by the param.
				Name:      "John Doe",
				Age:       30,
				CreatedAt: timestamppb.New(time.Unix(1758454323, 0)),
			},
		},
		{
			name: "support for different type of inputs",
			bytes: []byte(
				`{
					"id": "123",
					"name": "John Doe",
					"age": 30,
					"created_at": "2025-09-21T11:32:03.000Z",
					"emails": [
						"user.1@gmail.com",
						"user.2@gmail.com",
						"user3@gmail.com"
					],
					"scores": {
						"baseball": 10,
						"football": 2
					},
					"friends_loc": {
						"kevin": {
							"first_line": "21st street",
							"second_line": "34th street",
							"city": {
								"name": "New york",
								"pincode": 560015,
								"state": "New york",
								"country": "USA"
							}
						},
						"rachel": {
							"first_line": "12th street",
							"second_line": "42nd street",
							"city": {
								"name": "New hamshire",
								"pincode": 560015,
								"state": "New york",
								"country": "USA"
							}
						}
					},
					"my_addresses": [
						{
							"first_line": "21st street",
							"second_line": "34th street",
							"city": {
								"name": "New york",
								"pincode": 560015,
								"state": "New york",
								"country": "USA"
							}
						},
						{
							"first_line": "12th street",
							"second_line": "42nd street",
							"city": {
								"name": "New hamshire",
								"pincode": 560015,
								"state": "New york",
								"country": "USA"
							}
						}
					]
				}`,
			),
			options: &unmarshalOptions{
				timeFormat: ISOTimeFormat,
			},
			expected: &test.TestUser{
				Id:        "123", // should be overridden by the param.
				Name:      "John Doe",
				Age:       30,
				CreatedAt: timestamppb.New(time.Unix(1758454323, 0)),
				Emails:    []string{"user.1@gmail.com", "user.2@gmail.com", "user3@gmail.com"},
				Scores: map[string]int32{
					"baseball": 10,
					"football": 2,
				},
				FriendsLoc: map[string]*test.Address{
					"kevin": {
						FirstLine:  "21st street",
						SecondLine: "34th street",
						City: &test.City{
							Name:    "New york",
							Pincode: 560015,
							State:   "New york",
							Country: "USA",
						},
					},
					"rachel": {
						FirstLine:  "12th street",
						SecondLine: "42nd street",
						City: &test.City{
							Name:    "New hamshire",
							Pincode: 560015,
							State:   "New york",
							Country: "USA",
						},
					},
				},
				MyAddresses: []*test.Address{{
					FirstLine:  "21st street",
					SecondLine: "34th street",
					City: &test.City{
						Name:    "New york",
						Pincode: 560015,
						State:   "New york",
						Country: "USA",
					},
				}, {
					FirstLine:  "12th street",
					SecondLine: "42nd street",
					City: &test.City{
						Name:    "New hamshire",
						Pincode: 560015,
						State:   "New york",
						Country: "USA",
					},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := &test.TestUser{}
			err := unmarshalBytes(tt.bytes, actual, tt.params, tt.options)
			if tt.isErr {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}
			require.EqualExportedValues(t, tt.expected, actual)
		})
	}
}
