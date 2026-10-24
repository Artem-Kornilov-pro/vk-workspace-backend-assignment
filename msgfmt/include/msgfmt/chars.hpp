#pragma once

namespace msgfmt {

// Пробельный символ — кодовая точка со свойством Unicode White_Space.
bool is_whitespace(char32_t cp);

// Буква или цифра: ASCII плюс основные алфавиты Unicode (см. README).
bool is_letter_or_digit(char32_t cp);

// Служебные символы разметки: * _ ` [ ] ( ) \.
bool is_service_char(char32_t cp);

}  // namespace msgfmt
