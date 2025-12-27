// Hello World TitanAnvil Module (C++)
//
// This example module demonstrates the TitanAnvil C++ SDK by:
// - Getting the CPU make and model
// - Computing a SHA256 hash of the CPU information
// - Writing the results to a file in the temp directory
// - Returning the results

#include <titan/titan.h>
#include <sstream>

struct HelloWorldResult {
    std::string cpu_info;
    std::string hash;
    std::string file_path;
};

std::string to_json(const HelloWorldResult& result) {
    std::ostringstream oss;
    oss << "{"
        << "\"cpu_info\":\"" << titan::json::escape(result.cpu_info) << "\","
        << "\"hash\":\"" << result.hash << "\","
        << "\"file_path\":\"" << titan::json::escape(result.file_path) << "\""
        << "}";
    return oss.str();
}

int main() {
    try {
        titan::log::info("Hello World C++ module starting");

        // Get CPU information
        titan::log::info("Getting CPU information...");
        std::string cpu_info = titan::system::get_cpu_info();
        titan::log::info("CPU: " + cpu_info);

        // Compute SHA256 hash of CPU info
        titan::log::info("Computing SHA256 hash...");
        std::string hash = titan::crypto::sha256_string(cpu_info);
        titan::log::info("Hash: " + hash);

        // Determine temp directory path (cross-platform)
        std::string temp_dir;
        #ifdef _WIN32
        temp_dir = "C:\\Windows\\Temp";
        #else
        temp_dir = "/tmp";
        #endif

        std::string file_path = temp_dir + "/hello-from-titananvil-cpp.txt";

        // Create file contents
        std::ostringstream contents;
        contents << "CPU: " << cpu_info << "\n"
                 << "SHA256: " << hash << "\n";

        // Write to file
        titan::log::info("Writing to file: " + file_path);
        titan::fs::write_string(file_path, contents.str());
        titan::log::info("File written successfully");

        // Return results as JSON
        HelloWorldResult result{cpu_info, hash, file_path};
        std::string json_result = to_json(result);

        titan::log::info("Result: " + json_result);
        titan::log::info("Hello World C++ module completed successfully");

        return 0;
    } catch (const titan::Error& e) {
        titan::log::error(std::string("Error: ") + e.what());
        return 1;
    } catch (const std::exception& e) {
        titan::log::error(std::string("Unexpected error: ") + e.what());
        return 1;
    }
}
