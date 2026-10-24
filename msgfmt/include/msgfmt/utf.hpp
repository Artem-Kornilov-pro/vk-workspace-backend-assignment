#pragma once

#include <cstddef>
#include <optional>
#include <string>
#include <string_view>

namespace msgfmt {

// Декодирует UTF-8 в кодовые точки. Возвращает std::nullopt, если вход не
// является корректным UTF-8: обрыв последовательности, лишний байт
// продолжения, избыточная (overlong) запись, суррогат или значение > U+10FFFF.
std::optional<std::u32string> decode_utf8(std::string_view bytes);

// Дописывает кодовую точку в конец строки в кодировке UTF-8.
void append_utf8(std::string& out, char32_t cp);

std::string encode_utf8(std::u32string_view text);

// Сколько кодовых единиц UTF-16 занимает кодовая точка: вне BMP — суррогатная пара.
constexpr std::size_t utf16_length(char32_t cp) {
    return cp >= 0x10000 ? 2 : 1;
}

}  // namespace msgfmt
