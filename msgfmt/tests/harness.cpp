#include "harness.hpp"

#include <exception>
#include <iostream>
#include <vector>

namespace test {

namespace {

struct TestCase {
    std::string suite;
    std::string name;
    TestFn fn;
};

std::vector<TestCase>& registry() {
    static std::vector<TestCase> tests;
    return tests;
}

int g_failures = 0;
const TestCase* g_current = nullptr;

}  // namespace

bool register_test(const char* suite, const char* name, TestFn fn) {
    registry().push_back({suite, name, fn});
    return true;
}

void report_failure(const char* file, int line, const std::string& message) {
    ++g_failures;
    std::cerr << file << ':' << line << ": " << g_current->suite << '.' << g_current->name
              << " FAILED\n    " << message << '\n';
}

std::string show(std::u32string_view text) {
    std::ostringstream out;
    out << "U\"";
    for (const char32_t cp : text) {
        if (cp >= 0x20 && cp < 0x7F) {
            out << static_cast<char>(cp);
        } else {
            out << "\\x{" << std::hex << static_cast<unsigned long>(cp) << std::dec << '}';
        }
    }
    out << '"';
    return out.str();
}

}  // namespace test

// Использование: msgfmt_tests [suite]. Без аргумента запускаются все наборы.
int main(int argc, char* argv[]) {
    const std::string filter = argc > 1 ? argv[1] : "";
    int run = 0;
    for (const auto& tc : test::registry()) {
        if (!filter.empty() && tc.suite != filter) {
            continue;
        }
        ++run;
        test::g_current = &tc;
        try {
            tc.fn();
        } catch (const std::exception& e) {
            test::report_failure(__FILE__, __LINE__, std::string("exception: ") + e.what());
        }
    }
    if (run == 0) {
        std::cerr << "no tests matched '" << filter << "'\n";
        return 1;
    }
    std::cout << run << " tests, " << test::g_failures << " failures\n";
    return test::g_failures == 0 ? 0 : 1;
}
