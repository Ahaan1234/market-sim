cmake_minimum_required(VERSION 3.16)
project(market-sim)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_compile_options(-Wall -Wextra -Wpedantic)

add_executable(quantsim
    src/main.cpp
    src/PriceEngine.cpp
    src/OrderBook.cpp
    src/Trader.cpp
    src/Simulation.cpp
    src/agents/MakerTrader.cpp
    src/agents/TakerTrader.cpp
    src/agents/WhaleTrader.cpp
)
