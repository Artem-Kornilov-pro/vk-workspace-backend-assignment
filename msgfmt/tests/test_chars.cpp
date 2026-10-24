#include "harness.hpp"
#include "msgfmt/chars.hpp"

using msgfmt::is_letter_or_digit;
using msgfmt::is_service_char;
using msgfmt::is_whitespace;

TEST(chars, whitespace) {
    for (const char32_t cp : std::u32string_view(U" \t\n\v\f\r\x85\xA0\x1680\x2000\x200A\x2028\x2029\x202F\x205F\x3000")) {
        CHECK(is_whitespace(cp));
    }
    for (const char32_t cp : std::u32string_view(U"a_*Я💅\x200B\xFEFF")) {
        CHECK(!is_whitespace(cp));
    }
}

TEST(chars, letters_and_digits) {
    for (const char32_t cp : std::u32string_view(U"azAZ09ЯяЁёéßΩאع中あア한ｚ")) {
        CHECK(is_letter_or_digit(cp));
    }
    for (const char32_t cp : std::u32string_view(U" _*`[]()\\!.,-—«»💅\x00A0\x0482")) {
        CHECK(!is_letter_or_digit(cp));
    }
}

TEST(chars, service_chars) {
    for (const char32_t cp : std::u32string_view(U"*_`[]()\\")) {
        CHECK(is_service_char(cp));
    }
    for (const char32_t cp : std::u32string_view(U"a {}<>#~-+!Я")) {
        CHECK(!is_service_char(cp));
    }
}
