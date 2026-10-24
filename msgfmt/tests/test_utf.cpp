#include <string>

#include "harness.hpp"
#include "msgfmt/utf.hpp"

using msgfmt::decode_utf8;

namespace {

bool is_invalid(std::string_view bytes) {
    return !decode_utf8(bytes).has_value();
}

}  // namespace

TEST(utf, decodes_empty_input) {
    CHECK_EQ(decode_utf8("").value(), std::u32string());
}

TEST(utf, decodes_all_sequence_lengths) {
    CHECK_EQ(decode_utf8("a").value(), std::u32string(U"a"));
    CHECK_EQ(decode_utf8("\xD0\xAF").value(), std::u32string(U"Я"));
    CHECK_EQ(decode_utf8("\xE2\x82\xAC").value(), std::u32string(U"€"));
    CHECK_EQ(decode_utf8("\xF0\x9F\x92\x85").value(), std::u32string(U"💅"));
    CHECK_EQ(decode_utf8("1\xD0\xAF\xE2\x82\xAC\xF0\x9F\x92\x85").value(),
             std::u32string(U"1Я€💅"));
}

TEST(utf, decodes_boundary_code_points) {
    CHECK_EQ(decode_utf8(std::string("\x00", 1)).value(), std::u32string(1, U'\0'));
    CHECK_EQ(decode_utf8("\x7F").value(), std::u32string(U"\x7F"));
    CHECK_EQ(decode_utf8("\xC2\x80").value(), std::u32string(U"\x80"));
    CHECK_EQ(decode_utf8("\xDF\xBF").value(), std::u32string(U"\x7FF"));
    CHECK_EQ(decode_utf8("\xE0\xA0\x80").value(), std::u32string(U"\x800"));
    CHECK_EQ(decode_utf8("\xED\x9F\xBF").value(), std::u32string(U"\xD7FF"));
    CHECK_EQ(decode_utf8("\xEE\x80\x80").value(), std::u32string(U"\xE000"));
    CHECK_EQ(decode_utf8("\xEF\xBF\xBF").value(), std::u32string(U"\xFFFF"));
    CHECK_EQ(decode_utf8("\xF0\x90\x80\x80").value(), std::u32string(U"\x10000"));
    CHECK_EQ(decode_utf8("\xF4\x8F\xBF\xBF").value(), std::u32string(U"\x10FFFF"));
}

TEST(utf, rejects_unexpected_continuation_bytes) {
    CHECK(is_invalid("\x80"));
    CHECK(is_invalid("a\xBF"));
    CHECK(is_invalid("\xD0\xAF\xAF"));
}

TEST(utf, rejects_invalid_lead_bytes) {
    CHECK(is_invalid("\xF8\x88\x80\x80\x80"));
    CHECK(is_invalid("\xFE"));
    CHECK(is_invalid("\xFF"));
}

TEST(utf, rejects_truncated_sequences) {
    CHECK(is_invalid("\xD0"));
    CHECK(is_invalid("\xE2\x82"));
    CHECK(is_invalid("\xF0\x9F\x92"));
    CHECK(is_invalid("\xE2\x82z"));
    CHECK(is_invalid("\xD0 "));
}

TEST(utf, rejects_overlong_encodings) {
    CHECK(is_invalid("\xC0\x80"));
    CHECK(is_invalid("\xC1\xBF"));
    CHECK(is_invalid("\xE0\x80\x80"));
    CHECK(is_invalid("\xE0\x9F\xBF"));
    CHECK(is_invalid("\xF0\x80\x80\x80"));
    CHECK(is_invalid("\xF0\x8F\xBF\xBF"));
}

TEST(utf, rejects_surrogates) {
    CHECK(is_invalid("\xED\xA0\x80"));
    CHECK(is_invalid("\xED\xBF\xBF"));
    CHECK(is_invalid("\xED\xA0\xBD\xED\xB2\x85"));  // суррогатная пара в стиле CESU-8
}

TEST(utf, rejects_code_points_above_max) {
    CHECK(is_invalid("\xF4\x90\x80\x80"));
    CHECK(is_invalid("\xF5\x80\x80\x80"));
    CHECK(is_invalid("\xF7\xBF\xBF\xBF"));
}

TEST(utf, encodes_back_to_same_bytes) {
    const std::string samples[] = {"", "plain", "Привет, мир!", "💅 ок", "\xF4\x8F\xBF\xBF"};
    for (const auto& sample : samples) {
        CHECK_EQ(msgfmt::encode_utf8(decode_utf8(sample).value()), sample);
    }
}

TEST(utf, counts_utf16_units) {
    CHECK_EQ(msgfmt::utf16_length(U'a'), 1u);
    CHECK_EQ(msgfmt::utf16_length(U'Я'), 1u);
    CHECK_EQ(msgfmt::utf16_length(U'\xFFFF'), 1u);
    CHECK_EQ(msgfmt::utf16_length(U'💅'), 2u);
    CHECK_EQ(msgfmt::utf16_length(U'\x10000'), 2u);
}
