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

The basic use case uses the default ASCII lexer to split a string into sub-strings:

shlex.Split("one \"two three\" four") -> []string{"one", "two three", "four"}

To process a stream of strings:

l := NewLexer(os.Stdin)
for token, err := l.Next(); err != nil {
	// process token
}

To access the raw token stream (which includes tokens for comments):

t := NewTokenizer(os.Stdin)
for token, err := t.Next(); err != nil {
	// process token
}

*/
import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// TokenType is a top-level token classification: A word, space, comment, unknown.
type TokenType int

// runeTokenClass is the type of a UTF-8 character classification: A character, quote, space, escape.
type runeTokenClass int

// the internal state used by the lexer state machine
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

// Named classes of UTF-8 runes
const (
	charRunes             = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-,|"
	spaceRunes            = " \t\r\n"
	escapingQuoteRunes    = "\""
	nonEscapingQuoteRunes = "'"
	escapeRunes           = "\\"
	commentRunes          = "#"
)

// Classes of rune token
const (
	unknownRuneClass          runeTokenClass = iota
	charRuneClass             runeTokenClass = iota
	spaceRuneClass            runeTokenClass = iota
	escapingQuoteRuneClass    runeTokenClass = iota
	nonEscapingQuoteRuneClass runeTokenClass = iota
	escapeRuneClass           runeTokenClass = iota
	commentRuneClass          runeTokenClass = iota
	eofRuneClass              runeTokenClass = iota
)

// Classes of lexographic token
const (
	UnknownToken TokenType = iota
	WordToken    TokenType = iota
	SpaceToken   TokenType = iota
	CommentToken TokenType = iota
)

// Lexer state machine states
const (
	// startState - no runes have been seen
	startState lexerState = iota

	// inWordState - processing regular runes in a word
	inWordState lexerState = iota

	// escapingState - we have just consumed an escape rune; the next rune is literal
	escapingState lexerState = iota

	// escapingQuotedState - we have just consumed an escape rune within a quoted string
	escapingQuotedState lexerState = iota

	// quotingEscapingState - we are within a quoted string that supports escaping ("...")
	quotingEscapingState lexerState = iota

	// quotingState - we are within a string that does not support escaping ('...')
	quotingState lexerState = iota

	// commentState - we are within a comment (everything following an unquoted or unescaped #
	commentState lexerState = iota
)

const (
	initialTokenCapacity int = 100
)

// tokenClassifier is used for classifying rune characters
type tokenClassifier struct {
	typeMap map[rune]runeTokenClass
}

func addRuneClass(typeMap *map[rune]runeTokenClass, runes string, tokenType runeTokenClass) {
	for _, runeChar := range runes {
		(*typeMap)[runeChar] = tokenType
	}
}

// NewDefaultClassifier creates a new classifier for ASCII characters.
func NewDefaultClassifier() *tokenClassifier {
	typeMap := map[rune]runeTokenClass{}
	addRuneClass(&typeMap, charRunes, charRuneClass)
	addRuneClass(&typeMap, spaceRunes, spaceRuneClass)
	addRuneClass(&typeMap, escapingQuoteRunes, escapingQuoteRuneClass)
	addRuneClass(&typeMap, nonEscapingQuoteRunes, nonEscapingQuoteRuneClass)
	addRuneClass(&typeMap, escapeRunes, escapeRuneClass)
	addRuneClass(&typeMap, commentRunes, commentRuneClass)
	return &tokenClassifier{
		typeMap: typeMap}
}

// ClassifyRune classifiees a rune
func (classifier *tokenClassifier) ClassifyRune(runeVal rune) runeTokenClass {
	return classifier.typeMap[runeVal]
}

// Lexer turns an input stream into a sequence of tokens. Whitespace and comments are skipped.
type Lexer struct {
	tokenizer *Tokenizer
}

// NewLexer creates a new lexer from an input stream.
func NewLexer(r io.Reader) *Lexer {

	tokenizer := NewTokenizer(r)
	return &Lexer{tokenizer: tokenizer}
}

// Next returns the next word, or an error. If there are no more words,
// the error will be io.EOF.
func (l *Lexer) Next() (string, error) {
	var token *Token
	var err error
	for {
		token, err = l.tokenizer.Next()
		if err != nil {
			return "", err
		}
		switch token.tokenType {
		case WordToken:
			{
				return token.value, nil
			}
		case CommentToken:
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
	classifier *tokenClassifier
}

// NewTokenizer creates a new tokenizer from an input stream.
func NewTokenizer(r io.Reader) *Tokenizer {
	input := bufio.NewReader(r)
	classifier := NewDefaultClassifier()
	return &Tokenizer{
		input:      input,
		classifier: classifier}
}

// scanStream scans the stream for the next token using the internal state machine.
// It will panic if it encounters a rune which it does not know how to handle.
func (t *Tokenizer) scanStream() (*Token, error) {
	state := startState
	var tokenType TokenType
	value := make([]rune, 0, initialTokenCapacity)
	var (
		nextRune     rune
		nextRuneType runeTokenClass
		err          error
	)
SCAN:
	for {
		nextRune, _, err = t.input.ReadRune()
		nextRuneType = t.classifier.ClassifyRune(nextRune)
		if err != nil {
			if err == io.EOF {
				nextRuneType = eofRuneClass
				err = nil
			} else {
				return nil, err
			}
		}
		switch state {
		case startState: // no runes read yet
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						return nil, io.EOF
					}
				case charRuneClass:
					{
						tokenType = WordToken
						value = append(value, nextRune)
						state = inWordState
					}
				case spaceRuneClass:
					{
					}
				case escapingQuoteRuneClass:
					{
						tokenType = WordToken
						state = quotingEscapingState
					}
				case nonEscapingQuoteRuneClass:
					{
						tokenType = WordToken
						state = quotingState
					}
				case escapeRuneClass:
					{
						tokenType = WordToken
						state = escapingState
					}
				case commentRuneClass:
					{
						tokenType = CommentToken
						state = commentState
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case inWordState: // in a regular word
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						break SCAN
					}
				case charRuneClass, commentRuneClass:
					{
						value = append(value, nextRune)
					}
				case spaceRuneClass:
					{
						t.input.UnreadRune()
						break SCAN
					}
				case escapingQuoteRuneClass:
					{
						state = quotingEscapingState
					}
				case nonEscapingQuoteRuneClass:
					{
						state = quotingState
					}
				case escapeRuneClass:
					{
						state = escapingState
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case escapingState: // the rune after an escape character
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						err = fmt.Errorf("EOF found after escape character")
						break SCAN
					}
				case charRuneClass, spaceRuneClass, escapingQuoteRuneClass, nonEscapingQuoteRuneClass, escapeRuneClass, commentRuneClass:
					{
						state = inWordState
						value = append(value, nextRune)
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case escapingQuotedState: // the next rune after an escape character, in double quotes
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						err = fmt.Errorf("EOF found after escape character")
						break SCAN
					}
				case charRuneClass, spaceRuneClass, escapingQuoteRuneClass, nonEscapingQuoteRuneClass, escapeRuneClass, commentRuneClass:
					{
						state = quotingEscapingState
						value = append(value, nextRune)
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case quotingEscapingState: // in escaping double quotes
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						err = fmt.Errorf("EOF found when expecting closing quote")
						break SCAN
					}
				case charRuneClass, spaceRuneClass, nonEscapingQuoteRuneClass, commentRuneClass:
					{
						value = append(value, nextRune)
					}
				case escapingQuoteRuneClass:
					{
						state = inWordState
					}
				case escapeRuneClass:
					{
						state = escapingQuotedState
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case quotingState: // in non-escaping single quotes
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						err = fmt.Errorf("EOF found when expecting closing quote")
						break SCAN
					}
				case charRuneClass, spaceRuneClass, escapingQuoteRuneClass, escapeRuneClass, commentRuneClass:
					{
						value = append(value, nextRune)
					}
				case nonEscapingQuoteRuneClass:
					{
						state = inWordState
					}
				default:
					{
						return nil, fmt.Errorf("Uknown rune: %v", nextRune)
					}
				}
			}
		case commentState:
			{
				switch nextRuneType {
				case eofRuneClass:
					{
						break SCAN
					}
				case charRuneClass, escapingQuoteRuneClass, escapeRuneClass, commentRuneClass, nonEscapingQuoteRuneClass:
					{
						value = append(value, nextRune)
					}
				case spaceRuneClass:
					{
						if nextRune == '\n' {
							state = startState
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

// Next returns the next token in the stream.
func (t *Tokenizer) Next() (*Token, error) {
	return t.scanStream()
}

// Split partitions a string into a slice of strings.
func Split(s string) ([]string, error) {
	l := NewLexer(strings.NewReader(s))
	subStrings := make([]string, 0)
	for {
		word, err := l.Next()
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
