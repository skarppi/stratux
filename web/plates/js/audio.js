angular.module('appControllers').controller('AudioCtrl', AudioCtrl);      // get the main module contollers set
AudioCtrl.$inject = ['$scope', '$state', '$http'];                        // Inject my dependencies

// create our controller function with all necessary logic
function AudioCtrl($scope, $state, $http) {
	$scope.$parent.helppage = 'plates/audio-help.html';

	function connect($scope) {
		if (($scope === undefined) || ($scope === null))
			return; // we are getting called once after clicking away from the status page

		if (($scope.socket === undefined) || ($scope.socket === null)) {
			socket = new WebSocket(URL_STATUS_WS);
			$scope.socket = socket; // store socket in scope for enter/exit usage
		}

		$scope.ConnectState = "Disconnected";

		socket.onopen = function (msg) {
			// $scope.ConnectStyle = "label-success";
			$scope.ConnectState = "Connected";
		};

		socket.onclose = function (msg) {
			// $scope.ConnectStyle = "label-danger";
			$scope.ConnectState = "Disconnected";
			$scope.$apply();
			delete $scope.socket;
			setTimeout(function() {connect($scope);}, 1000);
		};

		socket.onerror = function (msg) {
			// $scope.ConnectStyle = "label-danger";
			$scope.ConnectState = "Error";
			$scope.$apply();
		};

		socket.onmessage = function (msg) {
			//console.log('Received status update.')

			var status = JSON.parse(msg.data)
			// Update Status
			$scope.AudioRecordingFile = status.AudioRecordingFile;
			$scope.AudioRecordingLoundness = Number(status.AudioRecordingLoundness).toFixed(1);

			$scope.loudness_percent = 100 + status.AudioRecordingLoundness;
			if ($scope.loudness_percent > 90) {
				$scope.loudness_color = "#FF0000";
			} else if ($scope.loudness_percent > 85) {
				$scope.loudness_color = "orange";
			} else if ($scope.loudness_percent > 50) {
				$scope.loudness_color = "green";
			} else {
				$scope.loudness_color = "blue";
			}

			$scope.$apply(); // trigger any needed refreshing of data
		};
	}

	$state.get('audio').onEnter = function () {
		// everything gets handled correctly by the controller
	};
	$state.get('audio').onExit = function () {
		if (($scope.socket !== undefined) && ($scope.socket !== null)) {
			$scope.socket.close();
			$scope.socket = null;
		}
	};

	connect($scope); // connect - opens a socket and listens for messages
}
