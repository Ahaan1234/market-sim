#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdlib>
#include <string>
#include <thread>
#include "Simulation.h"

static std::atomic<bool> g_running{true};

static void handleSignal(int) {
    g_running = false;
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

    Simulation sim(cfg);

    while (g_running) {
        sim.runTick();
        std::this_thread::sleep_for(std::chrono::milliseconds(500));
    }

    return 0;
}
