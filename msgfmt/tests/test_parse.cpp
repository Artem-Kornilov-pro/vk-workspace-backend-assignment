#include <cstddef>
#include <random>
#include <string>
#include <vector>

#include "harness.hpp"
#include "msgfmt/chars.hpp"
#include "msgfmt/format.hpp"
#include "msgfmt/json.hpp"
#include "msgfmt/utf.hpp"

using msgfmt::Document;
using msgfmt::Range;
using msgfmt::Style;

namespace {

std::string run(std::u32string_view input) {
    return msgfmt::to_json(msgfmt::parse(input));
}

std::string doc(std::string text, std::vector<Range> ranges = {}) {
    return msgfmt::to_json(Document{std::move(text), std::move(ranges)});
}

Range bold(std::size_t offset, std::size_t length) {
    return {Style::Bold, offset, length, ""};
}

Range italic(std::size_t offset, std::size_t length) {
    return {Style::Italic, offset, length, ""};
}

Range code(std::size_t offset, std::size_t length) {
    return {Style::Code, offset, length, ""};
}

Range link(std::size_t offset, std::size_t length, std::string url) {
    return {Style::Link, offset, length, std::move(url)};
}

std::size_t utf16_size(const std::string& utf8) {
    const std::u32string decoded = msgfmt::decode_utf8(utf8).value();
    std::size_t units = 0;
    for (const char32_t cp : decoded) {
        units += msgfmt::utf16_length(cp);
    }
    return units;
}

}  // namespace

// Приложение А, примеры 1–15.
TEST(parse, spec_examples) {
    CHECK_EQ(run(U"**важно**"), doc("важно", {bold(0, 5)}));
    CHECK_EQ(run(U"Привет, **мир**!"), doc("Привет, мир!", {bold(8, 3)}));
    CHECK_EQ(run(U"_курсив_"), doc("курсив", {italic(0, 6)}));
    CHECK_EQ(run(U"a_b_c"), doc("a_b_c"));
    CHECK_EQ(run(U"**не закрыто"), doc("**не закрыто"));
    CHECK_EQ(run(UR"(2 \* 3)"), doc("2 * 3"));
    CHECK_EQ(run(U"`чат * список`"), doc("чат * список", {code(0, 12)}));
    CHECK_EQ(run(U"[документация](https://docs.github.com)"),
             doc("документация", {link(0, 12, "https://docs.github.com")}));
    CHECK_EQ(run(U"**жирный _и_ курсив**"), doc("жирный и курсив", {bold(0, 15), italic(7, 1)}));
    CHECK_EQ(run(U"💅 **ок**"), doc("💅 ок", {bold(3, 2)}));
    CHECK_EQ(run(U"**💅 ок**"), doc("💅 ок", {bold(0, 5)}));
    CHECK_EQ(run(U"[текст]()"), doc("[текст]()"));
    CHECK_EQ(run(UR"(\\)"), doc("\\"));
    CHECK_EQ(run(U""), doc(""));
    CHECK_EQ(run(U"_ e_"), doc("_ e_"));
}

TEST(parse, plain_text_and_control_characters) {
    CHECK_EQ(run(U"просто текст"), doc("просто текст"));
    CHECK_EQ(run(U"строка 1\nстрока 2\t\x01"), doc("строка 1\nстрока 2\t\x01"));
    CHECK_EQ(run(U"**a\nb**"), doc("a\nb", {bold(0, 3)}));
}

TEST(parse, bold) {
    CHECK_EQ(run(U"2 ** 3"), doc("2 ** 3"));
    CHECK_EQ(run(U"** 3**"), doc("** 3**"));
    CHECK_EQ(run(U"**a **"), doc("**a **"));
    CHECK_EQ(run(U"**a** и **b**"), doc("a и b", {bold(0, 1), bold(4, 1)}));
    CHECK_EQ(run(U"****"), doc("****"));
    CHECK_EQ(run(U"***a***"), doc("*a*", {bold(0, 2)}));
    CHECK_EQ(run(U"*a*"), doc("*a*"));
    CHECK_EQ(run(U"** a**"), doc("** a**"));  // NBSP — пробельный символ
}

TEST(parse, italic_word_boundaries) {
    CHECK_EQ(run(U"snake_case_name"), doc("snake_case_name"));
    CHECK_EQ(run(U"слово_слово_"), doc("слово_слово_"));
    CHECK_EQ(run(U"_a_b"), doc("_a_b"));
    CHECK_EQ(run(U"_a_b_"), doc("a_b", {italic(0, 3)}));
    CHECK_EQ(run(U"(_a_)"), doc("(a)", {italic(1, 1)}));
    CHECK_EQ(run(U"_a_, _b_."), doc("a, b.", {italic(0, 1), italic(3, 1)}));
    CHECK_EQ(run(U"_a _"), doc("_a _"));
    CHECK_EQ(run(U"💅_ок_"), doc("💅ок", {italic(2, 2)}));
}

TEST(parse, code_is_verbatim) {
    CHECK_EQ(run(UR"(`**a** \_ [b](c)`)"), doc(R"(**a** \_ [b](c))", {code(0, 15)}));
    CHECK_EQ(run(UR"(`a\`)"), doc(R"(a\)", {code(0, 2)}));
    CHECK_EQ(run(U"` a`"), doc("` a`"));
    CHECK_EQ(run(U"`a `"), doc("`a `"));
    CHECK_EQ(run(U"``"), doc("``"));
    CHECK_EQ(run(U"`a` `b`"), doc("a b", {code(0, 1), code(2, 1)}));
}

TEST(parse, links) {
    CHECK_EQ(run(U"см. [ссылку](http://x.ru/a?b=c&d=(e)"), doc("см. ссылку", {link(4, 6, "http://x.ru/a?b=c&d=(e")}));
    CHECK_EQ(run(U"[a](b c)"), doc("[a](b c)"));
    CHECK_EQ(run(U"[a]"), doc("[a]"));
    CHECK_EQ(run(U"[a] (b)"), doc("[a] (b)"));
    CHECK_EQ(run(U"[ a](b)"), doc("[ a](b)"));
    CHECK_EQ(run(U"[a ](b)"), doc("[a ](b)"));
    CHECK_EQ(run(U"[](b)"), doc("[](b)"));
    CHECK_EQ(run(U"[a](b"), doc("[a](b"));
    CHECK_EQ(run(U"[a](b(c))"), doc("a)", {link(0, 1, "b(c")}));
    CHECK_EQ(run(UR"([a\]b](u))"), doc("a]b", {link(0, 3, "u")}));
    CHECK_EQ(run(UR"([a](u\v))"), doc("a", {link(0, 1, R"(u\v)")}));
    CHECK_EQ(run(U"[a ] [b](c)"), doc("[a ] b", {link(5, 1, "c")}));
}

TEST(parse, escapes) {
    CHECK_EQ(run(UR"(\*\*не жирный\*\*)"), doc("**не жирный**"));
    CHECK_EQ(run(UR"(\_\`\[\]\(\)\\)"), doc(R"(_`[]()\)"));
    CHECK_EQ(run(UR"(\a \n \)"), doc(R"(\a \n \)"));
    CHECK_EQ(run(UR"(\_a_)"), doc("_a_"));
    CHECK_EQ(run(UR"(**a\**)"), doc("**a**"));
    CHECK_EQ(run(UR"(**a\***)"), doc("a*", {bold(0, 2)}));
    CHECK_EQ(run(UR"(**a\\**)"), doc(R"(a\)", {bold(0, 2)}));
    CHECK_EQ(run(UR"(_a\_b_)"), doc("a_b", {italic(0, 3)}));
}

TEST(parse, nesting) {
    CHECK_EQ(run(U"_**a**_"), doc("a", {italic(0, 1), bold(0, 1)}));
    CHECK_EQ(run(U"**_a_**"), doc("a", {bold(0, 1), italic(0, 1)}));
    CHECK_EQ(run(U"**_a_ b**"), doc("a b", {bold(0, 3), italic(0, 1)}));
    CHECK_EQ(run(U"[**a** `b`](u)"), doc("a b", {link(0, 3, "u"), bold(0, 1), code(2, 1)}));
    CHECK_EQ(run(U"**[a](u)**"), doc("a", {bold(0, 1), link(0, 1, "u")}));
    CHECK_EQ(run(U"**a `b**` c**"), doc("a `b` c**", {bold(0, 4)}));
    CHECK_EQ(run(U"**a _b** c_"), doc("a _b c_", {bold(0, 4)}));
}

TEST(parse, same_style_inside_is_text) {
    CHECK_EQ(run(U"**a **b** c**"), doc("a **b c**", {bold(0, 5)}));
    CHECK_EQ(run(U"_a _b_ c_"), doc("a _b c_", {italic(0, 4)}));
    CHECK_EQ(run(U"[a [b](c) d](e)"), doc("a [b d](e)", {link(0, 4, "c")}));
}

TEST(parse, utf16_offsets) {
    CHECK_EQ(run(U"💅_ок_ 💅 [ссылка](u)"), doc("💅ок 💅 ссылка", {italic(2, 2), link(8, 6, "u")}));
    CHECK_EQ(run(U"**💅💅**`😀`"), doc("💅💅😀", {bold(0, 4), code(4, 2)}));
}

// Инварианты на случайных входах из служебных и обычных символов.
TEST(parse, random_inputs_keep_invariants) {
    const std::u32string alphabet = U"**__``[]()\\ \naЯ1💅";
    std::mt19937 rng(20261024);
    std::uniform_int_distribution<std::size_t> length_dist(0, 24);
    std::uniform_int_distribution<std::size_t> char_dist(0, alphabet.size() - 1);

    for (int iteration = 0; iteration < 20000; ++iteration) {
        std::u32string input(length_dist(rng), U' ');
        for (auto& cp : input) {
            cp = alphabet[char_dist(rng)];
        }
        const Document result = msgfmt::parse(input);
        const std::size_t text_units = utf16_size(result.text);

        for (std::size_t i = 0; i < result.ranges.size(); ++i) {
            const Range& a = result.ranges[i];
            CHECK(a.length > 0);
            CHECK(a.offset + a.length <= text_units);
            CHECK_EQ(a.url.empty(), a.style != Style::Link);
            for (const char32_t cp : msgfmt::decode_utf8(a.url).value_or(U")")) {
                CHECK(cp != U')' && !msgfmt::is_whitespace(cp));
            }
            if (i + 1 < result.ranges.size()) {
                const Range& b = result.ranges[i + 1];
                CHECK(a.offset < b.offset || (a.offset == b.offset && a.length >= b.length));
            }
            for (std::size_t j = i + 1; j < result.ranges.size(); ++j) {
                const Range& b = result.ranges[j];
                const bool disjoint = b.offset >= a.offset + a.length;
                const bool nested = b.offset + b.length <= a.offset + a.length;
                CHECK(disjoint || nested);
                CHECK(disjoint || a.style != b.style);
            }
        }

        // Экранированный текст разбирается обратно в себя без диапазонов.
        const std::string escaped = msgfmt::escape(input);
        const Document plain = msgfmt::parse(msgfmt::decode_utf8(escaped).value());
        CHECK_EQ(plain.text, msgfmt::encode_utf8(input));
        CHECK(plain.ranges.empty());
    }
}

// Входы, на которых наивный поиск закрывающего маркера квадратичен.
TEST(parse, large_inputs_are_linear) {
    auto repeat = [](std::u32string_view part, std::size_t times) {
        std::u32string out;
        out.reserve(part.size() * times);
        for (std::size_t i = 0; i < times; ++i) {
            out += part;
        }
        return out;
    };
    const std::size_t n = 200000;
    const std::u32string inputs[] = {
        repeat(U"_a ", n),
        repeat(U"**a ", n),
        repeat(U"`a ", n),
        repeat(U"[", n),
        repeat(U"[a] ", n),
        repeat(U"[", n) + U"a](" + repeat(U"b", n),
        repeat(U"\\", n),
        repeat(U"_**`[a", n),
        U"**" + repeat(U"_a [b ", n) + U"**",
    };
    for (const auto& input : inputs) {
        const Document result = msgfmt::parse(input);
        CHECK(!result.text.empty());
    }
    const Document many = msgfmt::parse(repeat(U"**a** ", n));
    CHECK_EQ(many.ranges.size(), n);
}
