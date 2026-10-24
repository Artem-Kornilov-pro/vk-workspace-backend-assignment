#include <string>

#include "harness.hpp"
#include "msgfmt/format.hpp"

using msgfmt::escape;

TEST(escape, example_from_spec) {
    CHECK_EQ(escape(U"2 * 3"), std::string(R"(2 \* 3)"));
}

TEST(escape, every_service_char) {
    CHECK_EQ(escape(U"*_`[]()\\"), std::string(R"(\*\_\`\[\]\(\)\\)"));
    CHECK_EQ(escape(U"**жирный** [ссылка](url)"),
             std::string(R"(\*\*жирный\*\* \[ссылка\]\(url\))"));
}

TEST(escape, keeps_other_text) {
    CHECK_EQ(escape(U""), std::string());
    CHECK_EQ(escape(U"Привет, мир! 💅\n#~{}<>"), std::string("Привет, мир! 💅\n#~{}<>"));
}
