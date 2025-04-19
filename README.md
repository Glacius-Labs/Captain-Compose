# Captain-Compose

Captain Compose is a simple deployment agent that applies `docker-compose.yaml` files using the Docker engine.

<div align="center">
  <img src="captain-compose.png" alt="Mascot" width="30%"/>
</div>

Unlike most tools, Captain Compose doesn't just run a compose file from the CLI—it listens to multiple input sources, such as MQTT or filesystem directories, and applies compose files automatically when they appear.

The idea is to make docker-compose deployable as a workload in larger orchestration systems, especially in distributed or edge environments.

This project is in early development and the design will evolve.
