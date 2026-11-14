package app

import "strings"

// Streamlined system for the romanization of Bulgarian (Law of 2009).
var latinOf = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sht", 'ъ': "a", 'ь': "y", 'ю': "yu", 'я': "ya",
}

func latin(s string) string {
	var b strings.Builder
	for _, r := range s {
		lower := []rune(strings.ToLower(string(r)))[0]
		l, ok := latinOf[lower]
		switch {
		case !ok:
			b.WriteRune(r)
		case lower != r: // capital: first letter upper, rest as is (Ts, Zh)
			b.WriteString(strings.ToUpper(l[:1]) + l[1:])
		default:
			b.WriteString(l)
		}
	}
	return b.String()
}
