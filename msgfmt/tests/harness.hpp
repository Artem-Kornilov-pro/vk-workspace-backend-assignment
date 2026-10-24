#pragma once

// Минимальный тестовый каркас на стандартной библиотеке: регистрация тестов
// по набору (suite), проверки CHECK/CHECK_EQ и запуск набора из командной строки.

#include <sstream>
#include <string>
#include <string_view>

namespace test {

using TestFn = void (*)();

bool register_test(const char* suite, const char* name, TestFn fn);
void report_failure(const char* file, int line, const std::string& message);

std::string show(std::u32string_view text);

inline std::string show(const std::u32string& text) {
    return show(std::u32string_view(text));
}

template <typename T>
std::string show(const T& value) {
    std::ostringstream out;
    out << value;
    return out.str();
}

}  // namespace test

#define TEST(suite, name)                                                                     \
    static void suite##_##name();                                                             \
    [[maybe_unused]] static const bool suite##_##name##_registered =                          \
        ::test::register_test(#suite, #name, &suite##_##name);                                \
    static void suite##_##name()

#define CHECK(cond)                                                                           \
    do {                                                                                      \
        if (!(cond)) {                                                                        \
            ::test::report_failure(__FILE__, __LINE__, "CHECK(" #cond ")");                   \
        }                                                                                     \
    } while (0)

#define CHECK_EQ(actual, expected)                                                            \
    do {                                                                                      \
        const auto actual_ = (actual);                                                        \
        const auto expected_ = (expected);                                                    \
        if (!(actual_ == expected_)) {                                                        \
            ::test::report_failure(__FILE__, __LINE__,                                        \
                                   "CHECK_EQ(" #actual ", " #expected ")\n    actual:   " +   \
                                       ::test::show(actual_) +                                \
                                       "\n    expected: " + ::test::show(expected_));         \
        }                                                                                     \
    } while (0)
