#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdlib>
#include <iostream>
#include <string>
#include <thread>
#include "OrderQueue.h"
#include "Protocol.h"
#include "Simulation.h"

// Must be file-scope so the signal handler can reach it.
static std::atomic<bool> g_running{true};

static void handleSignal(int) {
    g_running.store(false, std::memory_order_relaxed);
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

    OrderQueue orderQueue;

    // Background thread: reads JSON lines from stdin and pushes to the queue.
    // stdout is never touched from this thread.
    std::thread stdinThread([&]() {
        try {
            std::string line;
            while (g_running.load(std::memory_order_relaxed) &&
                   std::getline(std::cin, line)) {
                if (line.empty()) continue;
                auto order = protocol::parseIncomingOrder(line);
                if (order.has_value()) {
                    orderQueue.push(std::move(*order));
                }
                // silently discard malformed lines
            }
        } catch (...) {
            // A crashed stdin thread must not bring down the process.
        }
    });
    stdinThread.detach();

    Simulation sim(cfg, orderQueue);

    while (g_running.load(std::memory_order_relaxed)) {
        sim.runTick();
        std::this_thread::sleep_for(std::chrono::milliseconds(500));
    }

    return 0;
}
