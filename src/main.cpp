#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdlib>
#include <iostream>
#include <string>
#include <thread>
#include "OrderQueue.h"
#include "Simulation.h"

static std::atomic<bool> g_running{true};

static void handleSignal(int) {
    g_running = false;
}

// Parse a quoted string value for a given key from a JSON line.
// Returns "" if not found.
static std::string jsonString(const std::string& line, const std::string& key) {
    std::string needle = "\"" + key + "\":\"";
    auto pos = line.find(needle);
    if (pos == std::string::npos) return "";
    pos += needle.size();
    auto end = line.find('"', pos);
    if (end == std::string::npos) return "";
    return line.substr(pos, end - pos);
}

// Parse a numeric value for a given key from a JSON line.
// Returns 0.0 if not found.
static double jsonNumber(const std::string& line, const std::string& key) {
    std::string needle = "\"" + key + "\":";
    auto pos = line.find(needle);
    if (pos == std::string::npos) return 0.0;
    pos += needle.size();
    // skip whitespace
    while (pos < line.size() && line[pos] == ' ') ++pos;
    try { return std::stod(line.substr(pos)); }
    catch (...) { return 0.0; }
}

// Background thread: reads JSON lines from stdin and pushes to the queue.
// Expected format (one JSON object per line):
//   {"trader_id":"t1","order_id":"o1","side":"BUY","type":"LIMIT","price":100.5,"qty":2.0}
static void stdinReader(OrderQueue& queue) {
    std::string line;
    while (std::getline(std::cin, line)) {
        if (line.empty()) continue;

        std::string trader_id = jsonString(line, "trader_id");
        std::string order_id  = jsonString(line, "order_id");
        std::string sideStr   = jsonString(line, "side");
        std::string typeStr   = jsonString(line, "type");
        double      price     = jsonNumber(line, "price");
        double      qty       = jsonNumber(line, "qty");

        if (trader_id.empty() || order_id.empty()) continue;

        Side side;
        if (sideStr == "BUY")       side = Side::BUY;
        else if (sideStr == "SELL") side = Side::SELL;
        else continue;  // invalid_side — drop silently; Simulation validates qty/price

        OrderType type = (typeStr == "MARKET") ? OrderType::MARKET : OrderType::LIMIT;

        queue.push({trader_id, order_id, side, type, price, qty});
    }
}

int main(int argc, char* argv[]) {
    std::signal(SIGINT,  handleSignal);
    std::signal(SIGTERM, handleSignal);

    Simulation::Config cfg;

    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if (arg == "--makers" && i + 1 < argc)
            cfg.numMakers = std::stoi(argv[++i]);
        else if (arg == "--takers" && i + 1 < argc)
            cfg.numTakers = std::stoi(argv[++i]);
        else if (arg == "--whales" && i + 1 < argc)
            cfg.numWhales = std::stoi(argv[++i]);
    }

    OrderQueue queue;

    std::thread reader(stdinReader, std::ref(queue));
    reader.detach();

    Simulation sim(cfg, queue);

    while (g_running) {
        sim.runTick();
        std::this_thread::sleep_for(std::chrono::milliseconds(500));
    }

    return 0;
}
