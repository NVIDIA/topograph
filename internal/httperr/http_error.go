/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package httperr

type Error struct {
	code int
	msg  string
}

func NewError(code int, msg string) *Error {
	return &Error{
		code: code,
		msg:  msg,
	}
}

func (e *Error) Error() string {
	return e.msg
}

func (e *Error) Code() int {
	return e.code
}
