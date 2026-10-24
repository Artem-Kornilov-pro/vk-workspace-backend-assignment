#include <string>

#include "harness.hpp"
#include "msgfmt/json.hpp"

using msgfmt::Document;
using msgfmt::Style;

namespace {

std::string json_string(std::string_view value) {
    std::string out;
    msgfmt::append_json_string(out, value);
    return out;
}

}  // namespace

TEST(json, escapes_quote_and_backslash) {
    CHECK_EQ(json_string(R"(say "hi" \ bye)"), R"("say \"hi\" \\ bye")");
}

TEST(json, escapes_control_characters) {
    CHECK_EQ(json_string("a\nb\tc\rd\be\ff"), R"("a\nb\tc\rd\be\ff")");
    CHECK_EQ(json_string(std::string("\x00\x01\x1F", 3)), R"("\u0000\u0001\u001f")");
}

TEST(json, keeps_non_ascii_as_is) {
    CHECK_EQ(json_string("Привет 💅 /"), "\"Привет 💅 /\"");
    CHECK_EQ(json_string("\x7F"), "\"\x7F\"");
}

TEST(json, empty_document) {
    CHECK_EQ(msgfmt::to_json(Document{}), R"({"text":"","ranges":[]})");
}

TEST(json, document_with_ranges) {
    const Document doc{"жирный \"текст\"",
                       {{Style::Bold, 0, 15, ""},
                        {Style::Italic, 7, 1, ""},
                        {Style::Code, 1, 2, ""},
                        {Style::Link, 3, 4, "https://example.com/?q=\"x\""}}};
    CHECK_EQ(msgfmt::to_json(doc),
             R"({"text":"жирный \"текст\"","ranges":[)"
             R"({"style":"bold","offset":0,"length":15,"url":""},)"
             R"({"style":"italic","offset":7,"length":1,"url":""},)"
             R"({"style":"code","offset":1,"length":2,"url":""},)"
             R"({"style":"link","offset":3,"length":4,"url":"https://example.com/?q=\"x\""}]})");
}

TEST(json, error_object) {
    CHECK_EQ(msgfmt::error_json("invalid_utf8"), R"({"error":"invalid_utf8"})");
}
