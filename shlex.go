/*
Copyright 2012 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shlex

/*
Package shlex implements a simple lexer which splits input in to tokens using
shell-style rules for quoting and commenting.

TODO: document examples:

Alternative classifiers.
*/
import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// TokenType is a top-level token classification: A word, space, comment, unknown.
type TokenType int

// RuneTokenType is the type of a UTF-8 character classification: A character, quote, space, escape.
type RuneTokenType int

type lexerState int

// Token is a (type, value) pair representing a lexographical token.
type Token struct {
	tokenType TokenType
	value     string
}

// Equal reports whether tokens a, and b, are equal.
// Two tokens are equal if both their types and values are equal. A nil token can
// never be equal to another token.
func (a *Token) Equal(b *Token) bool {
	if a == nil || b == nil {
		return false
	}
	if a.tokenType != b.tokenType {
		return false
	}
	return a.value == b.value
}

// Named sets of UTF-8 runes
const (
	RUNE_CHAR              = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-,"
	RUNE_SPACE             = " \t\r\n"
	RUNE_ESCAPING_QUOTE    = "\""
	RUNE_NONESCAPING_QUOTE = "'"
	RUNE_ESCAPE            = "\\"
	RUNE_COMMENT           = "#"
)

// Classes of rune token
const (
	RUNETOKEN_UNKNOWN           RuneTokenType = iota
	RUNETOKEN_CHAR              RuneTokenType = iota
	RUNETOKEN_SPACE             RuneTokenType = iota
	RUNETOKEN_ESCAPING_QUOTE    RuneTokenType = iota
	RUNETOKEN_NONESCAPING_QUOTE RuneTokenType = iota
	RUNETOKEN_ESCAPE            RuneTokenType = iota
	RUNETOKEN_COMMENT           RuneTokenType = iota
	RUNETOKEN_EOF               RuneTokenType = iota
)

// Classes of lexographic token
const (
	TOKEN_UNKNOWN TokenType = iota
	TOKEN_WORD    TokenType = iota
	TOKEN_SPACE   TokenType = iota
	TOKEN_COMMENT TokenType = iota
)

// Lexer state machine states
const (
	STATE_START           lexerState = iota
	STATE_INWORD          lexerState = iota
	STATE_ESCAPING        lexerState = iota
	STATE_ESCAPING_QUOTED lexerState = iota
	STATE_QUOTED_ESCAPING lexerState = iota
	STATE_QUOTED          lexerState = iota
	STATE_COMMENT         lexerState = iota
)

const (
	INITIAL_TOKEN_CAPACITY int = 100
)

// TokenClassifier is used for classifying rune characters
type TokenClassifier struct {
	typeMap map[rune]RuneTokenType
}

func addRuneClass(typeMap *map[rune]RuneTokenType, runes string, tokenType RuneTokenType) {
	for _, rune := range runes {
		(*typeMap)[rune] = tokenType
	}
}

// NewDefaultClassifier creates a new classifier for ASCII characters.
func NewDefaultClassifier() *TokenClassifier {
	typeMap := map[rune]RuneTokenType{}
	addRuneClass(&typeMap, RUNE_CHAR, RUNETOKEN_CHAR)
	addRuneClass(&typeMap, RUNE_SPACE, RUNETOKEN_SPACE)
	addRuneClass(&typeMap, RUNE_ESCAPING_QUOTE, RUNETOKEN_ESCAPING_QUOTE)
	addRuneClass(&typeMap, RUNE_NONESCAPING_QUOTE, RUNETOKEN_NONESCAPING_QUOTE)
	addRuneClass(&typeMap, RUNE_ESCAPE, RUNETOKEN_ESCAPE)
	addRuneClass(&typeMap, RUNE_COMMENT, RUNETOKEN_COMMENT)
	return &TokenClassifier{
		typeMap: typeMap}
}

// ClassifyRune classifiees a rune
func (classifier *TokenClassifier) ClassifyRune(runeVal rune) RuneTokenType {
	return classifier.typeMap[runeVal]
}

// Lexer turns an input stream into a sequence of tokens. Whitespace and comments are skipped.
type Lexer struct {
	tokenizer *Tokenizer
}

// NewLexer creates a new lexer from an input stream.
func NewLexer(r io.Reader) (*Lexer, error) {

	tokenizer := NewTokenizer(r)
	lexer := &Lexer{tokenizer: tokenizer}
	return lexer, nil
}

// NextWords returns the next word, or an error. If there are no more words,
// the error will be io.EOF.
func (l *Lexer) NextWord() (string, error) {
	var token *Token
	var err error
	for {
		token, err = l.tokenizer.NextToken()
		if err != nil {
			return "", err
		}
		switch token.tokenType {
		case TOKEN_WORD:
			{
				return token.value, nil
			}
		case TOKEN_COMMENT:
			{
				// skip comments
			}
		default:
			{
				return "", fmt.Errorf("Unknown token type: %v", token.tokenType)
			}
		}
	}
	return "", io.EOF
}

// Tokenizer turns an input stream into a sequence of typed tokens
type Tokenizer struct {
	input      *bufio.Reader
	classifier *TokenClassifier
}

// NewTokenizer creates a new tokenizer from an input stream.
func NewTokenizer(r io.Reader) *Tokenizer {
	input := bufio.NewReader(r)
	classifier := NewDefaultClassifier()
	tokenizer := &Tokenizer{
		input:      input,
		classifier: classifier}
	return tokenizer
}

// scanStream scans the stream for the next token using the internal state machine.
// It will panic if it encounters a rune which it does not know how to handle.
// TODO: do not panic.
func (t *Tokenizer) scanStream() (*Token, error) {
	state := STATE_START
	var tokenType TokenType
	value := make([]rune, 0, INITIAL_TOKEN_CAPACITY)
	var (
		nextRune     rune
		nextRuneType RuneTokenType
		err          error
	)
SCAN:
	for {
		nextRune, _, err = t.input.ReadRune()
		nextRuneType = t.classifier.ClassifyRune(nextRune)
		if err != nil {
			if err == io.EOF {
				nextRuneType = RUNETOKEN_EOF
				err = nil
			} else {
				return nil, err
			}
		}
		switch state {
		case STATE_START: // no runes read yet
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						return nil, io.EOF
					}
				case RUNETOKEN_CHAR:
					{
						tokenType = TOKEN_WORD
						value = append(value, nextRune)
						state = STATE_INWORD
					}
				case RUNETOKEN_SPACE:
					{
					}
				case RUNETOKEN_ESCAPING_QUOTE:
					{
						tokenType = TOKEN_WORD
						state = STATE_QUOTED_ESCAPING
					}
				case RUNETOKEN_NONESCAPING_QUOTE:
					{
						tokenType = TOKEN_WORD
						state = STATE_QUOTED
					}
				case RUNETOKEN_ESCAPE:
					{
						tokenType = TOKEN_WORD
						state = STATE_ESCAPING
					}
				case RUNETOKEN_COMMENT:
					{
						tokenType = TOKEN_COMMENT
						state = STATE_COMMENT
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_INWORD: // in a regular word
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_COMMENT:
					{
						value = append(value, nextRune)
					}
				case RUNETOKEN_SPACE:
					{
						t.input.UnreadRune()
						break SCAN
					}
				case RUNETOKEN_ESCAPING_QUOTE:
					{
						state = STATE_QUOTED_ESCAPING
					}
				case RUNETOKEN_NONESCAPING_QUOTE:
					{
						state = STATE_QUOTED
					}
				case RUNETOKEN_ESCAPE:
					{
						state = STATE_ESCAPING
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_ESCAPING: // the rune after an escape character
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						err = fmt.Errorf("EOF found after escape character")
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_SPACE, RUNETOKEN_ESCAPING_QUOTE, RUNETOKEN_NONESCAPING_QUOTE, RUNETOKEN_ESCAPE, RUNETOKEN_COMMENT:
					{
						state = STATE_INWORD
						value = append(value, nextRune)
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_ESCAPING_QUOTED: // the next rune after an escape character, in double quotes
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						err = fmt.Errorf("EOF found after escape character")
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_SPACE, RUNETOKEN_ESCAPING_QUOTE, RUNETOKEN_NONESCAPING_QUOTE, RUNETOKEN_ESCAPE, RUNETOKEN_COMMENT:
					{
						state = STATE_QUOTED_ESCAPING
						value = append(value, nextRune)
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_QUOTED_ESCAPING: // in escaping double quotes
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						err = fmt.Errorf("EOF found when expecting closing quote")
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_SPACE, RUNETOKEN_NONESCAPING_QUOTE, RUNETOKEN_COMMENT:
					{
						value = append(value, nextRune)
					}
				case RUNETOKEN_ESCAPING_QUOTE:
					{
						state = STATE_INWORD
					}
				case RUNETOKEN_ESCAPE:
					{
						state = STATE_ESCAPING_QUOTED
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_QUOTED: // in non-escaping single quotes
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						err = fmt.Errorf("EOF found when expecting closing quote")
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_SPACE, RUNETOKEN_ESCAPING_QUOTE, RUNETOKEN_ESCAPE, RUNETOKEN_COMMENT:
					{
						value = append(value, nextRune)
					}
				case RUNETOKEN_NONESCAPING_QUOTE:
					{
						state = STATE_INWORD
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case STATE_COMMENT:
			{
				switch nextRuneType {
				case RUNETOKEN_EOF:
					{
						break SCAN
					}
				case RUNETOKEN_CHAR, RUNETOKEN_ESCAPING_QUOTE, RUNETOKEN_ESCAPE, RUNETOKEN_COMMENT, RUNETOKEN_NONESCAPING_QUOTE:
					{
						value = append(value, nextRune)
					}
				case RUNETOKEN_SPACE:
					{
						if nextRune == '\n' {
							state = STATE_START
							break SCAN
						} else {
							value = append(value, nextRune)
						}
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		default:
			{
				panic(fmt.Sprintf("Unexpected state: %v", state))
			}
		}
	}
	token := &Token{
		tokenType: tokenType,
		value:     string(value)}
	return token, err
}

// NextToken returns the next token in the stream.
func (t *Tokenizer) NextToken() (*Token, error) {
	return t.scanStream()
}

// Split partitions a string into a slice of strings.

func Split(s string) ([]string, error) {
	l, err := NewLexer(strings.NewReader(s))
	if err != nil {
		return nil, err
	}
	subStrings := make([]string, 0)
	for {
		word, err := l.NextWord()
		if err != nil {
			if err == io.EOF {
				return subStrings, nil
			}
			return subStrings, err
		}
		subStrings = append(subStrings, word)
	}
	return subStrings, nil
}
